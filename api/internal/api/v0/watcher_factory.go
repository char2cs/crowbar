package v0

import (
	"context"
	"fmt"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// newWatcherFactory builds the production watcherFactory. For each wsID it
// resolves the workspace row (worktree path + forkPointSha) via the repository,
// then constructs an unstarted *watch.Watcher wired to the dispatcher and the
// git status provider.
func newWatcherFactory(
	fsEngine enginefs.Engine,
	gitProvider enginefs.GitStatusProvider,
	workspace workspacerepo.Workspace,
	dispatcher enginefs.Dispatcher,
) watcherFactory {
	return func(
		ctx context.Context,
		wsID string,
	) (watcherProc, error) {
		ws, err := workspace.Get(ctx, wsID)
		if err != nil {
			return nil, fmt.Errorf("v0: watcher factory: get workspace %s: %w", wsID, err)
		}
		w := fsEngine.NewWatcher(
			ctx,
			wsID,
			ws.WorktreePath,
			ws.ForkPointSha,
			gitProvider,
			dispatcher,
		)
		return w, nil
	}
}
