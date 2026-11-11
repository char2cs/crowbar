package git

import (
	"context"
	"log/slog"
	"time"
)

// Push pushes the current branch and resyncs the working tree.
func (u *gitUsecase) Push(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Push(ctx, repoPath)
	})
}

// Fetch fetches from the remote and resyncs the working tree, then cascades the
// resync to the workspace's direct children (a fetch moves origin/<base>, which
// resolveDiffBase/summaryBase probe, so a child's diff base can shift).
func (u *gitUsecase) Fetch(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	if err := u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Fetch(ctx, repoPath)
	}); err != nil {
		return err
	}
	u.syncChildren(ctx, wsID, now)
	return nil
}

// Pull pulls with the given mode, resyncs, and cascades the resync to direct children, whose summaries
// measure against this branch's moved ref.
func (u *gitUsecase) Pull(
	ctx context.Context,
	wsID string,
	mode string,
	now time.Time,
) error {
	if err := u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Pull(ctx, repoPath, mode)
	}); err != nil {
		return err
	}
	u.syncChildren(ctx, wsID, now)
	return nil
}

// syncChildren resyncs every direct child's working-tree summary after a pull/fetch moved the parent tip
// they measure against. Best-effort: a failure is logged and never fails the pull or blocks a sibling.
// Grandchildren measure against their own parent's branch, which did not move.
func (u *gitUsecase) syncChildren(
	ctx context.Context,
	parentID string,
	now time.Time,
) {
	all, err := u.syncer.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "git: cascade child resync: list workspaces failed (continuing)",
			"parent", parentID, "err", err)
		return
	}
	for _, ws := range all {
		// A self-loop (ParentID == ID) must not resync the pulled workspace as its
		// own child, and only direct children of parentID are in scope.
		if ws.ParentID != parentID || ws.ID == parentID {
			continue
		}
		if _, err := u.syncer.SyncWorkingTreeState(ctx, ws.ID, now); err != nil {
			slog.WarnContext(ctx, "git: cascade child resync failed (continuing)",
				"parent", parentID, "child", ws.ID, "err", err)
		}
	}
}
