package git

import (
	"context"
	"fmt"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type WriteEngine interface {
	StageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error
	StageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error
	UnstageFile(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error
	UnstageHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
	) error
	Discard(
		ctx context.Context,
		repoPath string,
		filePath string,
	) error
	Commit(
		ctx context.Context,
		repoPath string,
		subject string,
		body string,
	) error
	Push(
		ctx context.Context,
		repoPath string,
	) error
	Fetch(
		ctx context.Context,
		repoPath string,
	) error
	Pull(
		ctx context.Context,
		repoPath string,
		mode string,
	) error
	CreateBranch(
		ctx context.Context,
		repoPath string,
		name string,
		source string,
		switchTo bool,
	) error
	RenameBranch(
		ctx context.Context,
		repoPath string,
		oldName string,
		newName string,
	) error
	DeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error
	SwitchBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error
	StashPush(
		ctx context.Context,
		repoPath string,
		message string,
	) error
	StashApply(
		ctx context.Context,
		repoPath string,
		id string,
	) error
	StashPop(
		ctx context.Context,
		repoPath string,
		id string,
	) error
	StashDrop(
		ctx context.Context,
		repoPath string,
		id string,
	) error
	Reset(
		ctx context.Context,
		repoPath string,
		mode string,
		commit string,
	) error
	Merge(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	Rebase(
		ctx context.Context,
		repoPath string,
		onto string,
	) error
	ResolveHunk(
		ctx context.Context,
		repoPath string,
		filePath string,
		hunkID string,
		resolution gitdomain.ConflictResolution,
		resolvedContent string,
	) error
	OperationContinue(
		ctx context.Context,
		repoPath string,
	) error
	OperationAbort(
		ctx context.Context,
		repoPath string,
	) error
}

type OpsEngine interface {
	ReadEngine
	WriteEngine
}

func (u *gitUsecase) mutate(
	ctx context.Context,
	wsID string,
	now time.Time,
	op func(repoPath string) error,
) error {
	repoPath, err := u.writePath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := op(repoPath); err != nil {
		return fmt.Errorf("git: mutate: %w", err)
	}
	if _, err := u.syncer.SyncWorkingTreeState(ctx, wsID, now); err != nil {
		return fmt.Errorf("git: resync: %w", err)
	}
	return nil
}

func (u *gitUsecase) StageFile(
	ctx context.Context,
	wsID string,
	filePath string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.StageFile(ctx, repoPath, filePath)
	})
}

func (u *gitUsecase) StageHunk(
	ctx context.Context,
	wsID string,
	filePath string,
	hunkID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.StageHunk(ctx, repoPath, filePath, hunkID)
	})
}

func (u *gitUsecase) UnstageFile(
	ctx context.Context,
	wsID string,
	filePath string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.UnstageFile(ctx, repoPath, filePath)
	})
}

func (u *gitUsecase) UnstageHunk(
	ctx context.Context,
	wsID string,
	filePath string,
	hunkID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.UnstageHunk(ctx, repoPath, filePath, hunkID)
	})
}

func (u *gitUsecase) Discard(
	ctx context.Context,
	wsID string,
	filePath string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Discard(ctx, repoPath, filePath)
	})
}

func (u *gitUsecase) Commit(
	ctx context.Context,
	wsID string,
	subject string,
	body string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Commit(ctx, repoPath, subject, body)
	})
}

func (u *gitUsecase) Reset(
	ctx context.Context,
	wsID string,
	mode string,
	commit string,
	now time.Time,
) error {
	if err := validateResetMode(mode); err != nil {
		return err
	}
	if err := validateOperand("commit", commit); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Reset(ctx, repoPath, mode, commit)
	})
}
