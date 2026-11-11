package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ClearBranch blanks an aggregate's Branch and touches nothing else. The consented
// detach of a repo home uses it so the home keeps its chats and threads rather
// than being deleted and recreated.
type ClearBranch struct {
	ID string
}

func (c ClearBranch) AggregateID() string {
	return c.ID
}

func (c ClearBranch) EventName() string {
	return "workspace.branch_cleared." + c.ID
}

func (c ClearBranch) ShouldSnapshot() bool {
	return false
}

func (c ClearBranch) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("clear branch: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ClearBranch) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Branch = ""
	return ws
}
