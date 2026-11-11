package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch records a branch rename and nothing else: a workspace's directory
// is fixed at creation, so there is no path to move, and children reference it
// by ID, so nothing re-parents.
type RenameBranch struct {
	ID     string
	Branch string
}

func (c RenameBranch) AggregateID() string {
	return c.ID
}

func (c RenameBranch) EventName() string {
	return "workspace.branch_renamed." + c.ID
}

func (c RenameBranch) ShouldSnapshot() bool {
	return true
}

func (c RenameBranch) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("rename branch: %w", asynxModels.ErrValidation)
	}
	if c.Branch == "" {
		return fmt.Errorf("rename branch: missing branch: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RenameBranch) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Branch = c.Branch
	return ws
}
