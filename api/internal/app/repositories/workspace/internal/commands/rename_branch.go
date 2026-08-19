package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch records a branch rename on an existing aggregate.
//
// It carries the branch and NOTHING ELSE. A workspace's directory is fixed at
// creation and never tracks the branch afterwards, so there is no path to move
// and none to record — which is the entire reason a rename is now one write
// instead of a directory move, a git repair and a compensating unwind.
//
// Identity, lineage and status are untouched — children reference this
// workspace by ID, not by branch, so a rename never re-parents anything.
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
