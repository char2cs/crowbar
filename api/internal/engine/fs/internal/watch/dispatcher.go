package watch

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type Dispatcher interface {
	OnFileChange(
		ctx context.Context,
		evt domain.FileChangeEvent,
	)
	OnGitStatus(
		ctx context.Context,
		wsID string,
		status gitdomain.GitStatus,
	)
	OnGitStatusUnreadable(
		ctx context.Context,
		wsID string,
		err error,
	)
	OnSyncWorkingTreeState(
		ctx context.Context,
		input SyncInput,
	)
}

type SyncInput struct {
	WsID         string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

type GitStatusProvider interface {
	ComputeStatus(
		ctx context.Context,
		repoPath string,
	) (gitdomain.GitStatus, error)
	ResolveDiffBase(
		ctx context.Context,
		repoPath string,
		base string,
	) gitdomain.DiffBase
	DiffStat(
		ctx context.Context,
		repoPath string,
		commit string,
	) (added, deleted int, err error)
}
