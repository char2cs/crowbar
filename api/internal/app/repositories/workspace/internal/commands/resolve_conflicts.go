package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResolveConflicts clears a workspace's pr-conflicts status once the in-progress
// operation in ITS OWN worktree — a kept rebase from a user-initiated "rebase
// onto parent", or a local merge/rebase — has been resolved and continued to a
// clean tree. pr-conflicts is otherwise sticky: SyncWorkingTreeState only ever
// SETS it and a provider sweep never overwrites it, so without this a resolved
// branch keeps showing the conflict warning. The PR badge is preserved when a PR
// exists (a later provider sweep refines the exact PR state); a non-conflict
// status is left untouched.
type ResolveConflicts struct {
	ID  string
	Now time.Time
}

func (c ResolveConflicts) AggregateID() string {
	return c.ID
}

func (c ResolveConflicts) EventName() string {
	return "workspace.conflictsResolved." + c.ID
}

func (c ResolveConflicts) ShouldSnapshot() bool {
	return false
}

func (c ResolveConflicts) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("resolve conflicts: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ResolveConflicts) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LastActivity = c.Now
	if ws.Status == domain.WorkspaceStatusPRConflicts {
		ws.Status = domain.WorkspaceStatusNew
		if ws.PRUrl != "" {
			ws.Status = domain.WorkspaceStatusPROpen
		}
	}
	return ws
}
