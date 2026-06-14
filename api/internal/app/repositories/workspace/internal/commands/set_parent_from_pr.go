package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetParentFromPR assigns ParentID based on an open PR's target branch without
// recomputing ForkPointSha. Only intended for provider-sync auto-wiring; the
// existing Reparent command is the user-facing reparent path.
type SetParentFromPR struct {
	ID       string
	ParentID string
}

func (c SetParentFromPR) AggregateID() string { return c.ID }

func (c SetParentFromPR) EventName() string {
	return "workspace.parent_set_from_pr." + c.ID
}

func (c SetParentFromPR) ShouldSnapshot() bool { return true }

func (c SetParentFromPR) Validate(current *domain.Workspace) error {
	if current == nil {
		return fmt.Errorf("set parent from pr: %w", asynxModels.ErrValidation)
	}
	if c.ParentID == "" {
		return fmt.Errorf("set parent from pr: missing parent id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetParentFromPR) EmitEvent(current *domain.Workspace) domain.Workspace {
	ws := *current
	ws.ParentID = c.ParentID
	return ws
}
