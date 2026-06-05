package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ClearPendingMerge removes the pendingMerge marker on continue/abort (07 §3.1).
type ClearPendingMerge struct {
	ID string
}

func (c ClearPendingMerge) AggregateID() string {
	return c.ID
}

func (c ClearPendingMerge) EventName() string {
	return "workspace.pending_merge_cleared." + c.ID
}

func (c ClearPendingMerge) ShouldSnapshot() bool {
	return false
}

func (c ClearPendingMerge) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("clear pending merge: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ClearPendingMerge) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.PendingMerge = nil
	return ws
}
