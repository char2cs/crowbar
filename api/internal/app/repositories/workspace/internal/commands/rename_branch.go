package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch records a branch rename on an existing aggregate: the new branch
// name AND the worktree path it moved to, which travel together because a
// managed worktree's leaf directory is derived from its branch name. Setting
// one without the other is what leaves the record disagreeing with git, so this
// command refuses a rename that carries only half of it.
//
// Identity, lineage and status are untouched — children reference this
// workspace by ID, not by branch, so a rename never re-parents anything.
type RenameBranch struct {
	ID           string
	Branch       string
	WorktreePath string
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
	if c.WorktreePath == "" {
		return fmt.Errorf("rename branch: missing worktree path: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RenameBranch) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Branch = c.Branch
	ws.WorktreePath = c.WorktreePath
	return ws
}
