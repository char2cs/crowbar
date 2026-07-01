package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ClearBranch blanks an existing aggregate's Branch to "" and touches nothing
// else. It is genuinely new capability: no other command mutates Branch
// (CreateWorkspace sets it once). Used by the consented Detach-holder op when
// the holder is the repo home, replacing the homeBranch="" blanking the old
// force-detaching adoptRepoHome did — without a delete-and-recreate that would
// drop the home aggregate's chats/threads (spec §3.5/§3.7/B6).
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
