package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch records a branch rename on an existing aggregate.
//
// It carries the branch and NOTHING else. The worktree used to travel with it,
// because the leaf directory was derived from the branch name and the two had to
// move together or the record would disagree with git. The root is keyed by the
// workspace id now, so a rename moves no directory at all — see Relocate for the
// one operation that does change a path.
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
