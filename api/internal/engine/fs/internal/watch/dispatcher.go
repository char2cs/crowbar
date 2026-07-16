package watch

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Dispatcher receives the fan-out callbacks from the file watcher (05 §5, 03 §5).
// The API/app layer provides the implementation that broadcasts and issues
// SyncWorkingTreeState commands.
type Dispatcher interface {
	// OnFileChange is called for every debounced filesystem event.
	OnFileChange(
		ctx context.Context,
		evt domain.FileChangeEvent,
	)
	// OnGitStatus is called after each GitStatus recompute.
	OnGitStatus(
		ctx context.Context,
		wsID string,
		status gitdomain.GitStatus,
	)
	// OnSyncWorkingTreeState is called when the derived workspace summary
	// (added/deleted/hasConflicts/hasCommits) has changed and the Workspace
	// aggregate must be updated.
	OnSyncWorkingTreeState(
		ctx context.Context,
		input SyncInput,
	)
}

// SyncInput carries the recomputed workspace summary fields for a
// SyncWorkingTreeState command (00 §5.3, 03 §5).
type SyncInput struct {
	WsID         string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

// GitStatusProvider recomputes GitStatus from the live git state.
// Implemented by the git engine; injected to avoid a circular import.
type GitStatusProvider interface {
	ComputeStatus(
		ctx context.Context,
		repoPath string,
	) (gitdomain.GitStatus, error)
	ComputeWorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		base string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)
}
