package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetLastError records the message from a failed background operation on the
// workspace aggregate (00 §4). The failure surfaces on the entity itself via
// the LastError field; it is never broadcast as a separate WS frame. Every
// subsequent successful mutating command clears LastError back to "".
type SetLastError struct {
	ID      string
	Message string
}

// AggregateID returns the workspace id this command targets.
func (c SetLastError) AggregateID() string {
	return c.ID
}

// EventName returns the unique event topic for this command.
func (c SetLastError) EventName() string {
	return "workspace.last_error_set." + c.ID
}

// ShouldSnapshot reports that this command does not force a snapshot: LastError
// is a small overlay that rides the next aggregate snapshot.
func (c SetLastError) ShouldSnapshot() bool {
	return false
}

// Validate rejects setting the error on a workspace that does not yet exist.
func (c SetLastError) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set last error: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent writes Message into the workspace's LastError field, leaving every
// other field untouched.
func (c SetLastError) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LastError = c.Message
	return ws
}
