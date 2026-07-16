package realtime

import (
	"context"
	"fmt"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// newWatcherFactory builds the production watcherFactory. For each wsID it
// resolves the workspace row (worktree path + diff base) via the repository,
// then constructs an unstarted *watch.Watcher wired to the dispatcher and the
// git status provider.
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
			return nil, fmt.Errorf("realtime: watcher factory: get workspace %s: %w", wsID, err)
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

// watcherDiffBase returns the ref the watcher's working-tree summary diffs
// against: the base BRANCH NAME — the parent's branch for a child, or the
// workspace's own branch for a protected root — so the git engine re-derives the
// merge-base with that branch's live tip on every recompute and the sidebar diff
// self-corrects as the base branch advances. Passing the stable branch name, not
// a SHA snapshotted at subscribe time, is what keeps the watcher-driven summary
// fresh for the whole life of the subscription.
//
// It falls back to the recorded ForkPointSha whenever the base branch is unusable
// — no branch name, an unresolvable parent row, or a branch that no longer
// resolves in the worktree — mirroring workspace.summaryBase so the watcher-driven
// summary and the on-demand summary can never diverge.
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
