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
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// legacyFuncs is the work behind the legacy post-open steps, the upgrades an
// install written by the pre-audit daemon needs. Each step pairs its work with
// the check that it left nothing it could have done
// (docs/plans/2026-09-24-upgrade-notes.md).
type legacyFuncs struct {
	createMissingHomes func(ctx context.Context, cp checkpoints) error
	missingHomes       func(ctx context.Context) ([]string, error)
	backfillChatTypes  func(ctx context.Context) (skipped int, err error)
	untypedChats       func(ctx context.Context) ([]string, error)
	restateProviders   func(ctx context.Context) (skipped int, err error)
	staleProviders     func(ctx context.Context) ([]string, error)
	// hookJournals is what the hook-deliveries step must remove, and why each
	// workspace it could not look in was left out.
	hookJournals func(ctx context.Context) (journals []hookJournal, unresolved []error, err error)
}

// checkpoints is the part of *migrate.Checkpoints the legacy steps use.
type checkpoints interface {
	Done(key string) bool
	Mark(key string, n migrate.Counts) error
}

func newLegacyFuncs(
	repos *repositories.Container,
	ucs *usecases.Container,
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
		restateProviders:  ucs.AgentRunner.RestateProvidersFromHistory,
		staleProviders:    ucs.AgentRunner.StaleProviders,
		hookJournals: func(ctx context.Context) ([]hookJournal, []error, error) {
			return retiredHookJournals(ctx, repos, ucs)
		},
	}
}

// Implemented reports whether this build implements migration step number, in
// either phase: what migrate.Inspect needs to preview a boot it cannot run here.
// It builds the impls over no stores, and runs none of them.
func Implemented(
	number int,
) bool {
	return migrations.PreOpenImpls()[number].Apply != nil || legacyImpls(legacyFuncs{})[number].Apply != nil
}

// legacyImpls implements the legacy post-open steps over f. Each adopts the
// marker an older daemon left for it instead of running.
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

// skipping is a row-by-row upgrade's Apply: the rows it could not replay are
// counted, not fatal, as they were before the engine.
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

// namedInHalt caps the rows a halt names: its message is kept in the ledger
// and served to the user.
const namedInHalt = 10

// leftNothing is a Verify that fails while left lists anything.
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

// homeCreator creates a project's home workspace under its deterministic id.
type homeCreator interface {
	CreateHome(ctx context.Context, project domain.Project) (domain.Workspace, error)
}

// reconcileProjectHomes gives a home to every project an older daemon saved
// without one: it minted homes lazily on the first read, and reads no longer
// write, so such a project's /home would stay 404.
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

// homeless lists the live projects with no live home workspace, except one
// whose home id a tombstone still holds: the home is not missing, and
// CreateHome cannot reuse the id until the purge frees it.
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

// hookDeliveriesDir is the retired exactly-once hook journal's directory,
// which the pre-audit daemon kept in every workspace's chats directory.
const hookDeliveriesDir = ".hook-deliveries"

// hookJournal is one retired journal and the workspace it was found under.
type hookJournal struct {
	workspaceID string
	path        string
}

// hookDeliveries deletes each retired journal journals lists. One that cannot
// be deleted is logged, counted and kept, as is a workspace that could not be
// looked in; the check then excuses exactly what this attempt kept.
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

// retiredHookJournals lists the retired journals still on disk in each
// workspace's Crowbar-managed chats directory, never inside a live checkout.
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
		// A journal that cannot even be stat'ed is listed: its removal reports why.
		if _, err := os.Lstat(journal); !errors.Is(err, fs.ErrNotExist) {
			found = append(found, hookJournal{workspaceID: ws.ID, path: journal})
		}
	}
	return found, unresolved, nil
}
