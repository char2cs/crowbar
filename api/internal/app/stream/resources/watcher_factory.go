package resources

import (
	"context"
	"fmt"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// newWatcherFactory builds the production watcherFactory: per wsID it resolves the
// workspace's worktree and diff base and builds an unstarted *watch.Watcher.
func newWatcherFactory(
	fsEngine enginefs.Engine,
	gitProvider enginefs.GitStatusProvider,
	workspace workspacerepo.Workspace,
	git enginegit.Engine,
	dispatcher enginefs.Dispatcher,
) watcherFactory {
	return func(
		ctx context.Context,
		wsID string,
	) (watcherProc, error) {
		ws, err := workspace.Get(ctx, wsID)
		if err != nil {
			return nil, fmt.Errorf("resources: watcher factory: get workspace %s: %w", wsID, err)
		}
		w := fsEngine.NewWatcher(
			ctx,
			wsID,
			ws.WorktreePath,
			watcherDiffBase(ctx, workspace, git, ws),
			gitProvider,
			dispatcher,
		)
		return w, nil
	}
}

// watcherDiffBase returns the base branch name (parent's for a child, own for a root)
// so the summary re-derives the merge-base with the live tip on each recompute. It falls
// back to ForkPointSha like workspace.summaryBase, so both summaries always agree.
func watcherDiffBase(
	ctx context.Context,
	workspace workspacerepo.Workspace,
	git enginegit.Engine,
	ws domain.Workspace,
) string {
	base := ws.Branch
	if ws.ParentID != "" {
		parent, err := workspace.Get(ctx, ws.ParentID)
		if err != nil {
			return ws.ForkPointSha
		}
		base = parent.Branch
	}
	if base == "" {
		return ws.ForkPointSha
	}
	if _, err := git.RevParse(ctx, ws.WorktreePath, base); err != nil {
		return ws.ForkPointSha
	}
	return base
}
