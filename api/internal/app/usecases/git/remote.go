package git

import (
	"context"
	"log/slog"
	"time"
)

func (u *gitUsecase) Push(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Push(ctx, repoPath)
	})
}

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
		if ws.ParentID != parentID || ws.ID == parentID {
			continue
		}
		if _, err := u.syncer.SyncWorkingTreeState(ctx, ws.ID, now); err != nil {
			slog.WarnContext(ctx, "git: cascade child resync failed (continuing)",
				"parent", parentID, "child", ws.ID, "err", err)
		}
	}
}
