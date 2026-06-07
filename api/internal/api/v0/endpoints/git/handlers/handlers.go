// Package handlers holds the gin handlers backing the git endpoint: the read
// routes (working-tree status dual-served with the live git stream, the
// paginated log, the working-tree or commit diff, the branch list, and the
// stash list — 02 §2.6) and the write routes (staging, commit, sync, branch,
// stash, and reset/merge/rebase mutations — 02 §2.7), plus the conflict and
// in-progress operation routes (the conflicting-file listing, hunk resolution,
// and operation continue/abort — 02 §2.8).
package handlers

import (
	"context"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Git is the git usecase surface the handlers need: the read half (status,
// diff, commit diff, log, branches, stashes) and the mutating half (staging,
// commit, sync, branch, stash, reset/merge/rebase). It mirrors the git usecase
// so the live usecase satisfies it directly.
type Git interface {
	Status(
		ctx context.Context,
		wsID string,
	) (gitdomain.GitStatus, error)
	Diff(
		ctx context.Context,
		wsID string,
		staged bool,
	) ([]gitdomain.FileDiff, error)
	CommitDiff(
		ctx context.Context,
		wsID string,
		sha string,
	) (gitdomain.MultiFileDiff, error)
	Log(
		ctx context.Context,
		wsID string,
		limit int,
		skip int,
	) ([]gitdomain.Commit, error)
	Branches(
		ctx context.Context,
		wsID string,
	) ([]gitdomain.Branch, error)
	Stashes(
		ctx context.Context,
		wsID string,
	) ([]gitdomain.Stash, error)
	ConflictedFiles(
		ctx context.Context,
		wsID string,
	) ([]string, error)
	ConflictHunks(
		ctx context.Context,
		wsID string,
		filePath string,
	) ([]gitdomain.ConflictHunk, error)
	GitWrite
}

// GitWrite is the mutating git surface the write handlers dispatch to: staging
// (file or hunk), discard, commit, sync (push/pull/fetch), branch lifecycle
// (create/rename/delete/switch), stash lifecycle, and reset/merge/rebase. Every
// method resyncs the working tree after the mutation.
type GitWrite interface {
	StageFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
	StageHunk(
		ctx context.Context,
		wsID string,
		filePath string,
		hunkID string,
		now time.Time,
	) error
	UnstageFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
	UnstageHunk(
		ctx context.Context,
		wsID string,
		filePath string,
		hunkID string,
		now time.Time,
	) error
	Discard(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
	Commit(
		ctx context.Context,
		wsID string,
		subject string,
		body string,
		now time.Time,
	) error
	Push(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
	Fetch(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
	Pull(
		ctx context.Context,
		wsID string,
		mode string,
		now time.Time,
	) error
	CreateBranch(
		ctx context.Context,
		wsID string,
		name string,
		source string,
		switchTo bool,
		now time.Time,
	) error
	RenameBranch(
		ctx context.Context,
		wsID string,
		oldName string,
		newName string,
		now time.Time,
	) error
	DeleteBranch(
		ctx context.Context,
		wsID string,
		name string,
		now time.Time,
	) error
	SwitchBranch(
		ctx context.Context,
		wsID string,
		name string,
		now time.Time,
	) error
	StashPush(
		ctx context.Context,
		wsID string,
		message string,
		now time.Time,
	) error
	StashApply(
		ctx context.Context,
		wsID string,
		id string,
		now time.Time,
	) error
	StashPop(
		ctx context.Context,
		wsID string,
		id string,
		now time.Time,
	) error
	StashDrop(
		ctx context.Context,
		wsID string,
		id string,
		now time.Time,
	) error
	Reset(
		ctx context.Context,
		wsID string,
		mode string,
		commit string,
		now time.Time,
	) error
	Merge(
		ctx context.Context,
		wsID string,
		branch string,
		now time.Time,
	) error
	Rebase(
		ctx context.Context,
		wsID string,
		onto string,
		now time.Time,
	) error
	ResolveHunk(
		ctx context.Context,
		wsID string,
		filePath string,
		hunkID string,
		resolution gitdomain.ConflictResolution,
		resolvedContent string,
		now time.Time,
	) error
	OperationContinue(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
	OperationAbort(
		ctx context.Context,
		wsID string,
		now time.Time,
	) error
}

// Handlers serves the /v0/workspaces/:wsId/git read and write routes from the
// git usecase. now supplies the timestamp for the working-tree resync that
// follows each mutation.
type Handlers struct {
	git Git
	now func() time.Time
}

// New builds the git Handlers from the git usecase and a clock. The clock backs
// the working-tree resync that every write route triggers.
func New(
	git Git,
	now func() time.Time,
) *Handlers {
	return &Handlers{
		git: git,
		now: now,
	}
}
