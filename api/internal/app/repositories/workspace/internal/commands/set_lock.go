package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// SetLock records the user's lock decision and applies it to the status in the same
// event. It outranks the provider's protected-branch locking for this workspace, and
// lives on the aggregate because writes, renames and merges enforce it daemon-side.
type SetLock struct {
	ID string
	// Locked is the user's answer, or nil to hand the question back to the
	// provider (the branch reverts to locked iff it is protected).
	Locked *bool
	// Protected is the provider's current answer, needed only to resolve the
	// status when Locked is nil.
	Protected bool
}

func (c SetLock) AggregateID() string {
	return c.ID
}

func (c SetLock) EventName() string {
	return "workspace.lock_set." + c.ID
}

func (c SetLock) ShouldSnapshot() bool {
	return true
}

func (c SetLock) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set lock: %w", asynxModels.ErrValidation)
	}
	// The home workspace is not a git worktree and has no branch to protect, so
	// there is nothing for a lock to mean on it.
	if current.Kind == domain.WorkspaceKindHome {
		return refusedLock("The project's home checkout has no branch to lock.")
	}
	// A placeholder is locked because another checkout holds its branch; unlocking
	// would promise writes to a directory that does not exist.
	if current.Provisioning == domain.WorkspacePlaceholder {
		return refusedLock("This branch has no worktree of its own, so it can't be locked or unlocked.")
	}
	return nil
}

// refusedLock is a lock change the workspace cannot take, worded for the user it
// is shown to.
func refusedLock(why string) error {
	return &problem.Error{Code: problem.Unprocessable, Message: why, Err: asynxModels.ErrValidation}
}

func (c SetLock) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LockOverride = c.Locked
	ws.Status = nextLockStatus(ws.Status, c.Locked, c.Protected)
	return ws
}
