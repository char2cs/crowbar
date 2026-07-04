// Package handlers contains the HTTP handler layer for v0 git endpoints.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Git is the git usecase surface consumed by the HTTP handlers.
type Git interface {
	// Reads
	Status(ctx context.Context, wsID string) (gitdomain.GitStatus, error)
	Diff(ctx context.Context, wsID string, staged bool) ([]gitdomain.FileDiff, error)
	Log(ctx context.Context, wsID string, limit int, skip int) ([]gitdomain.Commit, error)
	Blame(ctx context.Context, wsID string, filePath string) ([]gitdomain.BlameEntry, error)
	Branches(ctx context.Context, wsID string) ([]gitdomain.Branch, error)
	Stashes(ctx context.Context, wsID string) ([]gitdomain.Stash, error)
	ConflictedFiles(ctx context.Context, wsID string) ([]string, error)
	ConflictHunks(ctx context.Context, wsID string, filePath string) ([]gitdomain.ConflictHunk, error)
	CommitDiff(ctx context.Context, wsID string, sha string) (gitdomain.MultiFileDiff, error)
	// Writes
	StageFile(ctx context.Context, wsID string, filePath string, now time.Time) error
	StageHunk(ctx context.Context, wsID string, filePath string, hunkID string, now time.Time) error
	UnstageFile(ctx context.Context, wsID string, filePath string, now time.Time) error
	UnstageHunk(ctx context.Context, wsID string, filePath string, hunkID string, now time.Time) error
	Discard(ctx context.Context, wsID string, filePath string, now time.Time) error
	Commit(ctx context.Context, wsID string, subject string, body string, now time.Time) error
	Push(ctx context.Context, wsID string, now time.Time) error
	Fetch(ctx context.Context, wsID string, now time.Time) error
	Pull(ctx context.Context, wsID string, mode string, now time.Time) error
	CreateBranch(ctx context.Context, wsID string, name string, source string, switchTo bool, now time.Time) error
	RenameBranch(ctx context.Context, wsID string, oldName string, newName string, now time.Time) error
	DeleteBranch(ctx context.Context, wsID string, name string, now time.Time) error
	SwitchBranch(ctx context.Context, wsID string, name string, now time.Time) error
	StashPush(ctx context.Context, wsID string, message string, now time.Time) error
	StashApply(ctx context.Context, wsID string, id string, now time.Time) error
	StashPop(ctx context.Context, wsID string, id string, now time.Time) error
	StashDrop(ctx context.Context, wsID string, id string, now time.Time) error
	Reset(ctx context.Context, wsID string, mode string, commit string, now time.Time) error
	Merge(ctx context.Context, wsID string, branch string, now time.Time) error
	Rebase(ctx context.Context, wsID string, onto string, now time.Time) error
	ResolveHunk(ctx context.Context, wsID string, filePath string, hunkID string, resolution gitdomain.ConflictResolution, resolvedContent string, now time.Time) error
	OperationContinue(ctx context.Context, wsID string, now time.Time) error
	OperationAbort(ctx context.Context, wsID string, now time.Time) error
}

// LastErrorSetter records the message from a failed background git op on the
// workspace entity so the failure is delivered on the workspace WebSocket stream
// (00 §4: errors live on the entity, never a separate WS frame). It is satisfied
// by the workspace repository, wired in router.go.
type LastErrorSetter interface {
	SetLastError(
		ctx context.Context,
		id string,
		message string,
	) (domain.Workspace, error)
}

// WorkSignal brackets a workspace's background git-op window so the daemon
// serves a real Working overlay on the workspace entity (spinner in the tree).
// It is satisfied by the repositories container, wired in router.go.
type WorkSignal interface {
	BeginWork(
		ctx context.Context,
		wsID string,
	)
	EndWork(
		ctx context.Context,
		wsID string,
	)
}

// Handlers holds the git usecase, the workspace error sink for slow-op failures,
// the working-overlay signal, and exposes HTTP handler methods.
type Handlers struct {
	git        Git
	lastErrors LastErrorSetter
	working    WorkSignal
}

// New builds a Handlers from the git usecase, the workspace error sink that
// surfaces slow-op async failures on the workspace entity, and the
// working-overlay signal that marks the entity while a slow git op runs.
func New(
	git Git,
	lastErrors LastErrorSetter,
	working WorkSignal,
) *Handlers {
	return &Handlers{
		git:        git,
		lastErrors: lastErrors,
		working:    working,
	}
}
