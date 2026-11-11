package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Delete tombstones a workspace (Status "deleted") with no IO and no Forget: the
// "workspace.deleted.*" reactor does the teardown off the write path. Other fields
// are kept so the boot orphan sweep can still read them off the tombstone.
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
