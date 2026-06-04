package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateWorkspace creates a new workspace aggregate seeded to status "new".
type CreateWorkspace struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Locked        bool
	MergeStrategy domain.MergeStrategy
	Now           time.Time
}

func (c CreateWorkspace) AggregateID() string {
	return c.ID
}

func (c CreateWorkspace) EventName() string {
	return "workspace.created." + c.ID
}

func (c CreateWorkspace) ShouldSnapshot() bool {
	return true
}

func (c CreateWorkspace) Validate(
	current *domain.Workspace,
) error {
	if current != nil {
		return fmt.Errorf("create workspace: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.RepoID == "" || c.ProjectID == "" {
		return fmt.Errorf("create workspace: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateWorkspace) EmitEvent(
	_ *domain.Workspace,
) domain.Workspace {
	strategy := c.MergeStrategy
	if strategy == "" {
		strategy = domain.MergeStrategyMerge
	}
	return domain.Workspace{
		ID:            c.ID,
		RepoID:        c.RepoID,
		ProjectID:     c.ProjectID,
		Branch:        c.Branch,
		WorktreePath:  c.WorktreePath,
		ForkPointSha:  c.ForkPointSha,
		ParentID:      c.ParentID,
		Status:        domain.WorkspaceStatusNew,
		Locked:        c.Locked,
		MergeStrategy: strategy,
		LastActivity:  c.Now,
		CreatedAt:     c.Now,
	}
}
