package git

import (
	"context"
	"fmt"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Merge merges a branch and resyncs the working tree. branch is validated so a
// leading-dash value can never be read as a git option (argument injection).
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

// Rebase rebases the current branch and resyncs the working tree. onto is
// validated so a leading-dash value can never be read as a git option such as
// `--exec=<cmd>` (argument injection → arbitrary command execution).
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

// ResolveHunk resolves a conflict hunk and resyncs the working tree.
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

// OperationContinue finalizes an in-progress merge/rebase/pull and resyncs. Success means no conflict is
// left in this worktree, so the sticky pr-conflicts status is cleared (a merge-into-parent conflict is
// resolved by the parent's continue instead).
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

// OperationAbort aborts an in-progress merge/rebase/pull and resyncs.
func (u *gitUsecase) OperationAbort(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.OperationAbort(ctx, repoPath)
	})
}
