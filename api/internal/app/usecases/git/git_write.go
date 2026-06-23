package git

import (
	"context"
	"fmt"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WriteEngine is the mutating surface the git usecase passes through to.
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

// OpsEngine is the full git operation surface the git usecase consumes.
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

// StageFile stages a file and resyncs the working tree.
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

// StageHunk stages a hunk and resyncs the working tree.
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

// UnstageFile unstages a file and resyncs the working tree.
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

// UnstageHunk unstages a hunk and resyncs the working tree.
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

// Discard discards changes to a file and resyncs the working tree.
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

// Commit creates a commit and resyncs the working tree.
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

// Fetch fetches from the remote and resyncs the working tree.
func (u *gitUsecase) Fetch(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Fetch(ctx, repoPath)
	})
}

// Pull pulls with the given mode and resyncs the working tree.
func (u *gitUsecase) Pull(
	ctx context.Context,
	wsID string,
	mode string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Pull(ctx, repoPath, mode)
	})
}

// CreateBranch creates a branch and resyncs the working tree.
func (u *gitUsecase) CreateBranch(
	ctx context.Context,
	wsID string,
	name string,
	source string,
	switchTo bool,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.CreateBranch(ctx, repoPath, name, source, switchTo)
	})
}

// RenameBranch renames a branch and resyncs the working tree.
func (u *gitUsecase) RenameBranch(
	ctx context.Context,
	wsID string,
	oldName string,
	newName string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.RenameBranch(ctx, repoPath, oldName, newName)
	})
}

// DeleteBranch deletes a branch and resyncs the working tree.
func (u *gitUsecase) DeleteBranch(
	ctx context.Context,
	wsID string,
	name string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.DeleteBranch(ctx, repoPath, name)
	})
}

// SwitchBranch checks out a branch and resyncs the working tree.
func (u *gitUsecase) SwitchBranch(
	ctx context.Context,
	wsID string,
	name string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.SwitchBranch(ctx, repoPath, name)
	})
}

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

// Reset runs git reset and resyncs the working tree.
func (u *gitUsecase) Reset(
	ctx context.Context,
	wsID string,
	mode string,
	commit string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Reset(ctx, repoPath, mode, commit)
	})
}

// Merge merges a branch and resyncs the working tree.
func (u *gitUsecase) Merge(
	ctx context.Context,
	wsID string,
	branch string,
	now time.Time,
) error {
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.Merge(ctx, repoPath, branch)
	})
}

// Rebase rebases the current branch and resyncs the working tree.
func (u *gitUsecase) Rebase(
	ctx context.Context,
	wsID string,
	onto string,
	now time.Time,
) error {
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

// OperationContinue finalizes an in-progress merge/rebase/pull and resyncs.
//
// A successful continue fully completes the operation in THIS worktree (git
// errors the continue if any conflict remains), so any pr-conflicts status the
// branch carried from its own kept rebase is now resolved. pr-conflicts is
// otherwise sticky, so clear it here. This is safe for the merge-into-parent
// case: that marks the CHILD pr-conflicts while the markers live in the PARENT,
// and it is resolved via the PARENT's continue — a child never reaches this path
// with a clean-but-still-logically-conflicting state.
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
