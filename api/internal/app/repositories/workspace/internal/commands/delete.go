package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Delete tombstones a workspace by folding its Status to the terminal "deleted"
// value (spec §3.8 delete lifecycle). It is a PURE command: it emits
// "workspace.deleted.<id>" and does NO fs/git IO and does NOT Forget the
// aggregate. The store projection persists the "deleted" row and the async
// reactor subscribed on topic "workspace.deleted.*" (spec §3.6) performs the
// physical teardown (cascade thread Forgets + rm -rf worktree +
// axWorkspace.Forget) off the synchronous write path. Every other field is
// preserved so the boot orphan-sweep can still read project_id/repo_id/worktree
// off the tombstone row.
type Delete struct {
	ID string
}

// AggregateID returns the workspace id this command targets.
func (c Delete) AggregateID() string {
	return c.ID
}

// EventName returns the terminal delete event topic for this workspace.
func (c Delete) EventName() string {
	return "workspace.deleted." + c.ID
}

// ShouldSnapshot reports that a tombstone does not force a snapshot: the
// aggregate is terminal and is Forgotten by the delete reactor.
func (c Delete) ShouldSnapshot() bool {
	return false
}

// Validate rejects deleting a workspace that does not exist.
func (c Delete) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("delete workspace: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent folds the current aggregate to Status = deleted, preserving every
// other field so the read-model tombstone retains its location data.
func (c Delete) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Status = domain.WorkspaceStatusDeleted
	return ws
}
