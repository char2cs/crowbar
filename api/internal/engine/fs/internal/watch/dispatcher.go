package watch

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
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
		status domain.GitStatus,
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
	) (domain.GitStatus, error)
	ComputeWorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		forkPointSha string,
	) (added int, deleted int, hasConflicts bool, hasCommits bool, err error)
}
