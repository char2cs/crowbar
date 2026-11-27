package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	"github.com/char2cs/crowbar/api/internal/app/migratetree"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type legacyFuncs struct {
	createMissingHomes func(ctx context.Context, cp checkpoints) error
	missingHomes       func(ctx context.Context) ([]string, error)
	backfillChatTypes  func(ctx context.Context) (skipped int, err error)
	untypedChats       func(ctx context.Context) ([]string, error)
	restateProviders   func(ctx context.Context) (skipped int, err error)
	staleProviders     func(ctx context.Context) ([]string, error)
	hookJournals       func(ctx context.Context) (journals []hookJournal, unresolved []error, err error)
}

type checkpoints interface {
	Done(key string) bool
	Mark(key string, n migrate.Counts) error
}

func newLegacyFuncs(
	repos *repositories.Container,
	ucs *usecases.Container,
	h home.Home,
) legacyFuncs {
	return legacyFuncs{
		createMissingHomes: func(ctx context.Context, cp checkpoints) error {
			projects, workspaces, err := projectsAndWorkspaces(ctx, repos, ucs)
			if err != nil {
				return err
			}
			return reconcileProjectHomes(ctx, projects, workspaces, ucs.ProjectImport, cp)
		},
		missingHomes: func(ctx context.Context) ([]string, error) {
			projects, workspaces, err := projectsAndWorkspaces(ctx, repos, ucs)
			if err != nil {
				return nil, err
			}
			return projectIDs(homeless(projects, workspaces)), nil
		},
		backfillChatTypes: ucs.AgentChat.BackfillChatTypes,
		untypedChats:      ucs.AgentChat.UntypedChats,
		restateProviders: func(ctx context.Context) (int, error) {
			newest, err := chatledger.NewestProviders(ctx, h)
			if err != nil {
				return 0, err
			}
			return ucs.AgentRunner.RestateProvidersFromHistory(ctx, newest)
		},
		staleProviders: func(ctx context.Context) ([]string, error) {
			newest, err := chatledger.NewestProviders(ctx, h)
			if err != nil {
				return nil, err
			}
			return ucs.AgentRunner.StaleProviders(ctx, newest)
		},
		hookJournals: func(ctx context.Context) ([]hookJournal, []error, error) {
			return retiredHookJournals(ctx, repos, ucs)
		},
	}
}

func Implemented(
	number int,
) bool {
	return migrations.PreOpenImpls()[number].Apply != nil ||
		postOpenImpls(legacyFuncs{}, nil, migratetree.LegacySources{})[number].Apply != nil
}

func postOpenImpls(
	f legacyFuncs,
	ports *repotree.Ports,
	src migratetree.LegacySources,
) migrate.Impls {
	impls := legacyImpls(f)
	impls[migrations.StepPlacementTree] = migratetree.PlacementTreeImpl(ports, src)
	return impls
}

func legacyImpls(
	f legacyFuncs,
) migrate.Impls {
	return migrate.Impls{
		migrations.StepProjectHomes: migrations.Adopting(migrations.StepProjectHomes, migrate.Impl{
			Apply: func(ctx context.Context, _ migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
				return nil, f.createMissingHomes(ctx, cp)
			},
			Verify: leftNothing("projects without a home", f.missingHomes),
		}),
		migrations.StepChatTypes: migrations.Adopting(migrations.StepChatTypes, migrate.Impl{
			Apply:  skipping(f.backfillChatTypes),
			Verify: leftNothing("replayable chats without a type", f.untypedChats),
		}),
		migrations.StepChatProviders: migrations.Adopting(migrations.StepChatProviders, migrate.Impl{
			Apply:  skipping(f.restateProviders),
			Verify: leftNothing("replayable chats on a provider their history disagrees with", f.staleProviders),
		}),
		migrations.StepHookDeliveries: migrations.Adopting(migrations.StepHookDeliveries, hookDeliveries(f.hookJournals)),
	}
}

func skipping(
	upgrade func(ctx context.Context) (int, error),
) func(context.Context, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error) {
	return func(ctx context.Context, _ migrate.Env, _ *migrate.Checkpoints) (migrate.Counts, error) {
		skipped, err := upgrade(ctx)
		return skippedRows(skipped), err
	}
}

func skippedRows(
	n int,
) migrate.Counts {
	if n == 0 {
		return nil
	}
	return migrate.Counts{"skipped_rows": int64(n)}
}

const namedInHalt = 10

func leftNothing(
	what string,
	left func(ctx context.Context) ([]string, error),
) func(context.Context, migrate.Env) error {
	return func(ctx context.Context, _ migrate.Env) error {
		ids, err := left(ctx)
		if err != nil || len(ids) == 0 {
			return err
		}
		named := strings.Join(ids[:min(len(ids), namedInHalt)], ", ")
		if len(ids) > namedInHalt {
			named += fmt.Sprintf(" and %d more", len(ids)-namedInHalt)
		}
		return fmt.Errorf("%d %s remain: %s", len(ids), what, named)
	}
}

func projectsAndWorkspaces(
	ctx context.Context,
	repos *repositories.Container,
	ucs *usecases.Container,
) ([]domain.Project, []domain.Workspace, error) {
	projects, err := ucs.Project.List(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list projects: %w", err)
	}
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list workspaces: %w", err)
	}
	return projects, workspaces, nil
}

type homeCreator interface {
	CreateHome(ctx context.Context, project domain.Project) (domain.Workspace, error)
}

func reconcileProjectHomes(
	ctx context.Context,
	projects []domain.Project,
	workspaces []domain.Workspace,
	creator homeCreator,
	cp checkpoints,
) error {
	for _, p := range homeless(projects, workspaces) {
		if _, err := creator.CreateHome(ctx, p); err != nil {
			return fmt.Errorf("create the home of project %s: %w", p.ID, err)
		}
		if err := cp.Mark(p.ID, migrate.Counts{"homes_created": 1}); err != nil {
			return err
		}
		slog.InfoContext(ctx, "app: created a project's missing home", "project_id", p.ID)
	}
	return nil
}

func homeless(
	projects []domain.Project,
	workspaces []domain.Workspace,
) []domain.Project {
	homed := make(map[string]bool, len(workspaces))
	taken := make(map[string]bool, len(workspaces))
	for _, ws := range workspaces {
		taken[ws.ID] = true
		if ws.Kind == domain.WorkspaceKindHome && ws.Status != domain.WorkspaceStatusDeleted {
			homed[ws.ProjectID] = true
		}
	}
	var out []domain.Project
	for _, p := range projects {
		if !p.Deleting && !homed[p.ID] && !taken[workspace.ProjectHomeID(p.ID)] {
			out = append(out, p)
		}
	}
	return out
}

func projectIDs(
	projects []domain.Project,
) []string {
	ids := make([]string, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	return ids
}

const hookDeliveriesDir = ".hook-deliveries"

type hookJournal struct {
	workspaceID string
	path        string
}

func hookDeliveries(
	journals func(ctx context.Context) ([]hookJournal, []error, error),
) migrate.Impl {
	var kept map[string]bool
	return migrate.Impl{
		Apply: func(ctx context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
			found, unresolved, err := journals(ctx)
			if err != nil {
				return nil, err
			}
			for _, cause := range unresolved {
				slog.WarnContext(ctx, "app: hook journals: skipped a workspace", "err", cause)
			}
			kept = map[string]bool{}
			for _, j := range found {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				if err := worktreepath.RemoveUnderHome(env.Home.Root(), j.path); err != nil {
					slog.WarnContext(ctx, "app: hook journals: kept one", "workspace_id", j.workspaceID, "err", err)
					kept[j.path] = true
					continue
				}
				if err := cp.Mark(j.workspaceID, migrate.Counts{"dirs_removed": 1}); err != nil {
					return nil, err
				}
			}
			return skippedRows(len(unresolved) + len(kept)), nil
		},
		Verify: leftNothing("retired hook journals", func(ctx context.Context) ([]string, error) {
			found, _, err := journals(ctx)
			var left []string
			for _, j := range found {
				if !kept[j.path] {
					left = append(left, j.path)
				}
			}
			return left, err
		}),
	}
}

func retiredHookJournals(
	ctx context.Context,
	repos *repositories.Container,
	ucs *usecases.Container,
) ([]hookJournal, []error, error) {
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list workspaces: %w", err)
	}
	var found []hookJournal
	var unresolved []error
	seen := map[string]bool{}
	for _, ws := range workspaces {
		chatsDir, err := ucs.AgentWorkspaceReader.AgentChatsDir(ctx, ws.ID)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		if err != nil {
			unresolved = append(unresolved, fmt.Errorf("workspace %s: chats directory: %w", ws.ID, err))
			continue
		}
		if seen[chatsDir] || worktreepath.IsLiveCheckout(chatsDir) {
			continue
		}
		seen[chatsDir] = true
		journal := filepath.Join(chatsDir, hookDeliveriesDir)
		if _, err := os.Lstat(journal); !errors.Is(err, fs.ErrNotExist) {
			found = append(found, hookJournal{workspaceID: ws.ID, path: journal})
		}
	}
	return found, unresolved, nil
}
