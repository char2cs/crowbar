package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// upgradeStep is one one-off migration of what an older daemon left on disk.
// It reports whether it finished; an unfinished step runs again next boot.
type upgradeStep struct {
	name string
	run  func(ctx context.Context) bool
}

// runUpgrades runs, synchronously and before anything is served, every
// upgrade step this install has not yet finished. A finished step leaves an
// empty marker under <home>/state/upgrades, so an upgraded install pays for
// each step once. Every step is idempotent and best-effort per row.
func runUpgrades(
	ctx context.Context,
	crowbarHome string,
	steps []upgradeStep,
) {
	dir := filepath.Join(worktreepath.GlobalStateDir(crowbarHome), "upgrades")
	for _, step := range steps {
		marker := filepath.Join(dir, step.name)
		if _, err := os.Stat(marker); err == nil {
			continue
		}
		if !step.run(ctx) {
			slog.WarnContext(ctx, "app: upgrade step unfinished; the next boot retries", "step", step.name)
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			slog.WarnContext(ctx, "app: upgrade: record step", "step", step.name, "err", err)
			continue
		}
		if err := os.WriteFile(marker, nil, 0o600); err != nil {
			slog.WarnContext(ctx, "app: upgrade: record step", "step", step.name, "err", err)
		}
	}
}

// upgradeSteps lists the migrations an install written by the pre-audit
// daemon needs (docs/plans/2026-09-24-upgrade-notes.md).
func upgradeSteps(
	crowbarHome string,
	view *gormdb.DB,
	repos *repositories.Container,
	ucs *usecases.Container,
) []upgradeStep {
	steps := []upgradeStep{
		{name: "project-homes", run: func(ctx context.Context) bool {
			return createMissingProjectHomes(ctx, repos, ucs)
		}},
		{name: "workspace-paths-table", run: func(ctx context.Context) bool {
			return dropRetiredTable(ctx, view, "workspace_paths")
		}},
	}
	if ucs.AgentChat == nil || ucs.AgentRunner == nil || ucs.AgentWorkspaceReader == nil {
		return steps
	}
	return append(steps,
		upgradeStep{name: "chat-types", run: ucs.AgentChat.BackfillChatTypes},
		upgradeStep{name: "chat-providers-from-history", run: ucs.AgentRunner.RestateProvidersFromHistory},
		upgradeStep{name: "hook-deliveries", run: func(ctx context.Context) bool {
			return removeRetiredHookDeliveries(ctx, crowbarHome, repos, ucs)
		}},
	)
}

// homeCreator creates a project's home workspace under its deterministic id.
type homeCreator interface {
	CreateHome(ctx context.Context, project domain.Project) (domain.Workspace, error)
}

// createMissingProjectHomes gives a home to every project an older daemon
// saved without one: it minted homes lazily on the first read, and reads no
// longer write, so such a project's /home would stay 404.
func createMissingProjectHomes(
	ctx context.Context,
	repos *repositories.Container,
	ucs *usecases.Container,
) bool {
	projects, err := ucs.Project.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: upgrade: list projects", "err", err)
		return false
	}
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: upgrade: list workspaces", "err", err)
		return false
	}
	return reconcileProjectHomes(ctx, projects, workspaces, ucs.ProjectImport)
}

func reconcileProjectHomes(
	ctx context.Context,
	projects []domain.Project,
	workspaces []domain.Workspace,
	creator homeCreator,
) bool {
	homed := make(map[string]bool, len(workspaces))
	for _, ws := range workspaces {
		if ws.Kind == domain.WorkspaceKindHome && ws.Status != domain.WorkspaceStatusDeleted {
			homed[ws.ProjectID] = true
		}
	}
	complete := true
	for _, p := range projects {
		if p.Deleting || homed[p.ID] {
			continue
		}
		if _, err := creator.CreateHome(ctx, p); err != nil {
			slog.ErrorContext(ctx, "app: upgrade: create a project's missing home", "project_id", p.ID, "err", err)
			complete = false
			continue
		}
		slog.InfoContext(ctx, "app: upgrade: created a project's missing home", "project_id", p.ID)
	}
	return complete
}

// hookDeliveriesDir is the retired exactly-once hook journal's directory,
// which the pre-audit daemon kept in every workspace's chats directory.
const hookDeliveriesDir = ".hook-deliveries"

// removeRetiredHookDeliveries deletes that directory from each workspace's
// Crowbar-managed chats directory, and nothing else.
func removeRetiredHookDeliveries(
	ctx context.Context,
	crowbarHome string,
	repos *repositories.Container,
	ucs *usecases.Container,
) bool {
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: upgrade: list workspaces", "err", err)
		return false
	}
	seen := map[string]bool{}
	for _, ws := range workspaces {
		chatsDir, err := ucs.AgentWorkspaceReader.AgentChatsDir(ctx, ws.ID)
		if err != nil || seen[chatsDir] || worktreepath.IsLiveCheckout(chatsDir) {
			continue
		}
		seen[chatsDir] = true
		worktreepath.RemoveUnderHome(ctx, crowbarHome, filepath.Join(chatsDir, hookDeliveriesDir))
	}
	return true
}

// dropRetiredTable drops a table no code reads any more.
func dropRetiredTable(
	ctx context.Context,
	view *gormdb.DB,
	table string,
) bool {
	if view == nil {
		return false
	}
	if err := view.WithContext(ctx).Migrator().DropTable(table); err != nil {
		slog.ErrorContext(ctx, "app: upgrade: drop retired table", "table", table, "err", err)
		return false
	}
	return true
}
