package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetLock records the user's own lock decision for a workspace and applies it
// to the status in the same event.
//
// Locking has always been the provider's call: a protected branch is created
// locked and re-locked on every poll (nextProviderStatus). This is the override
// on top of that — it does not replace the automatic locking, it outranks it for
// the one workspace the user has an opinion about. Any branch can be locked,
// including a fork child; any branch can be unlocked, including main.
//
// Locked is a real restriction, not a badge: file writes, branch renames, merges
// and several git operations all refuse a locked workspace. That is why the
// decision is persisted on the aggregate rather than kept in the sidebar — a
// lock the frontend remembered would be a lock the daemon did not enforce.
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
		return fmt.Errorf("set lock: home workspace cannot be locked: %w", asynxModels.ErrValidation)
	}
	// A placeholder is locked BECAUSE it has no worktree of its own — something
	// else is holding its branch. Unlocking it would promise write access to a
	// directory that does not exist. Retry provisioning is the way out of that
	// state, not this.
	if current.WorktreePath == "" {
		return fmt.Errorf("set lock: workspace has no worktree: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetLock) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	// A successful mutating command clears any stale background error (00 §4).
	ws.LastError = ""
	ws.LockOverride = c.Locked
	ws.Status = nextLockStatus(ws.Status, c.Locked, c.Protected)
	return ws
}
