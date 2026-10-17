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
	removeHookJournals func(ctx context.Context, cp checkpoints) (skipped int, err error)
}

// checkpoints is the part of *migrate.Checkpoints the legacy steps use.
type checkpoints interface {
	Done(key string) bool
	Mark(key string, n migrate.Counts) error
}

func newLegacyFuncs(
	crowbarHome string,
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
		removeHookJournals: func(ctx context.Context, cp checkpoints) (int, error) {
			return removeRetiredHookDeliveries(ctx, crowbarHome, repos, ucs, cp)
		},
	}
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
		migrations.StepHookDeliveries: migrations.Adopting(migrations.StepHookDeliveries, migrate.Impl{
			Apply: func(ctx context.Context, _ migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
				skipped, err := f.removeHookJournals(ctx, cp)
				return skippedRows(skipped), err
			},
		}),
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

// leftNothing is a Verify that fails while left lists anything.
func leftNothing(
	what string,
	left func(ctx context.Context) ([]string, error),
) func(context.Context, migrate.Env) error {
	return func(ctx context.Context, _ migrate.Env) error {
		ids, err := left(ctx)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			return fmt.Errorf("%d %s remain: %s", len(ids), what, strings.Join(ids, ", "))
		}
		return nil
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

// homeless lists the live projects with no live home workspace.
func homeless(
	projects []domain.Project,
	workspaces []domain.Workspace,
) []domain.Project {
	homed := make(map[string]bool, len(workspaces))
	for _, ws := range workspaces {
		if ws.Kind == domain.WorkspaceKindHome && ws.Status != domain.WorkspaceStatusDeleted {
			homed[ws.ProjectID] = true
		}
	}
	var out []domain.Project
	for _, p := range projects {
		if !p.Deleting && !homed[p.ID] {
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

// removeRetiredHookDeliveries deletes that directory from each workspace's
// Crowbar-managed chats directory, and nothing else. A workspace whose chats
// directory cannot be resolved is skipped.
func removeRetiredHookDeliveries(
	ctx context.Context,
	crowbarHome string,
	repos *repositories.Container,
	ucs *usecases.Container,
	cp checkpoints,
) (int, error) {
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces: %w", err)
	}
	skipped := 0
	seen := map[string]bool{}
	for _, ws := range workspaces {
		if cp.Done(ws.ID) {
			continue
		}
		chatsDir, err := ucs.AgentWorkspaceReader.AgentChatsDir(ctx, ws.ID)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return skipped, ctxErr
		}
		if err != nil {
			slog.WarnContext(ctx, "app: hook journals: skipped a workspace", "workspace_id", ws.ID, "err", err)
			skipped++
			continue
		}
		if seen[chatsDir] || worktreepath.IsLiveCheckout(chatsDir) {
			continue
		}
		seen[chatsDir] = true
		removed, err := removeHookJournal(ctx, crowbarHome, filepath.Join(chatsDir, hookDeliveriesDir))
		if err != nil {
			return skipped, err
		}
		if err := cp.Mark(ws.ID, migrate.Counts{"dirs_removed": removed}); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}

// removeHookJournal deletes journal, reporting 1 when it was there; one that
// is still there afterwards could not be deleted.
func removeHookJournal(
	ctx context.Context,
	crowbarHome string,
	journal string,
) (int64, error) {
	there, err := exists(journal)
	if err != nil || !there {
		return 0, err
	}
	worktreepath.RemoveUnderHome(ctx, crowbarHome, journal)
	if there, err = exists(journal); err != nil || there {
		return 0, errors.Join(fmt.Errorf("could not delete %s", journal), err)
	}
	return 1, nil
}

func exists(
	path string,
) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
}
