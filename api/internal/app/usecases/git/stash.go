package git

import (
	"context"
	"time"
)

func (u *gitUsecase) StashPush(
	ctx context.Context,
	wsID string,
	message string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.StashPush(ctx, repoPath, message)
	})
}

func (u *gitUsecase) StashApply(
	ctx context.Context,
	wsID string,
	id string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.StashApply(ctx, repoPath, id)
	})
}

func (u *gitUsecase) StashPop(
	ctx context.Context,
	wsID string,
	id string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.StashPop(ctx, repoPath, id)
	})
}

func (u *gitUsecase) StashDrop(
	ctx context.Context,
	wsID string,
	id string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.StashDrop(ctx, repoPath, id)
	})
}
