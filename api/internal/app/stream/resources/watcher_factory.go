package resources

import (
	"context"
	"fmt"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

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
