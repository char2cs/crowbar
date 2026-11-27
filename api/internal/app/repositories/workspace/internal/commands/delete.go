package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type Delete struct {
	ID string
}

func (c Delete) AggregateID() string {
	return c.ID
}

func (c Delete) EventName() string {
	return "workspace.deleted." + c.ID
}

func (c Delete) ShouldSnapshot() bool {
	return false
}

func (c Delete) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("delete workspace: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Delete) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Status = domain.WorkspaceStatusDeleted
	return ws
}
