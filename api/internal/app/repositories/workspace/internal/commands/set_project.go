package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetProject re-points a workspace at the project that now owns its repository.
// It exists for exactly one caller: moving a repo between projects. The repo row
// is the source of truth for that edge, but every workspace carries a denormalised
// ProjectID that the hierarchical routes and the WS namespace are keyed on, so a
// repo whose workspaces were left behind would keep them but stop showing them.
//
// It moves no worktree. The on-disk path was derived once, at create time, and is
// stored absolute in both the record and the id↔path index, so it keeps resolving
// from wherever it already is; only newly derived paths land under the new project.
type SetProject struct {
	ID        string
	ProjectID string
}

func (c SetProject) AggregateID() string {
	return c.ID
}

func (c SetProject) EventName() string {
	return "workspace.project_set." + c.ID
}

func (c SetProject) ShouldSnapshot() bool {
	return true
}

func (c SetProject) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set project: %w", asynxModels.ErrValidation)
	}
	if c.ProjectID == "" {
		return fmt.Errorf("set project: missing project: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetProject) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ProjectID = c.ProjectID
	return ws
}
