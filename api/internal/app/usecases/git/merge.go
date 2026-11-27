package git

import (
	"context"
	"fmt"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func (u *gitUsecase) Merge(
	ctx context.Context,
	wsID string,
	branch string,
	now time.Time,
) error {
	if err := validateOperand("branch", branch); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Merge(ctx, repoPath, branch)
	})
}

func (u *gitUsecase) Rebase(
	ctx context.Context,
	wsID string,
	onto string,
	now time.Time,
) error {
	if err := validateOperand("onto", onto); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Rebase(ctx, repoPath, onto)
	})
}

func (u *gitUsecase) ResolveHunk(
	ctx context.Context,
	wsID string,
	filePath string,
	hunkID string,
	resolution gitdomain.ConflictResolution,
	resolvedContent string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.ResolveHunk(ctx, repoPath, filePath, hunkID, resolution, resolvedContent)
	})
}

func (u *gitUsecase) OperationContinue(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	if err := u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.OperationContinue(ctx, repoPath)
	}); err != nil {
		return err
	}
	if _, err := u.syncer.ResolveConflicts(ctx, wsID, now); err != nil {
		return fmt.Errorf("git: resolve conflicts: %w", err)
	}
	return nil
}

func (u *gitUsecase) OperationAbort(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.OperationAbort(ctx, repoPath)
	})
}
