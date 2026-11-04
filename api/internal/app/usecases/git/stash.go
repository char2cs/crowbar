package git

import (
	"context"
	"time"
)

// StashPush stashes the current changes and resyncs the working tree.
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

// StashApply applies a stash and resyncs the working tree.
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

// StashPop applies and removes a stash and resyncs the working tree.
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

// StashDrop removes a stash and resyncs the working tree.
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
