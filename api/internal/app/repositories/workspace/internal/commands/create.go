package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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
	Protected     bool
	IsDefault     bool
	MergeStrategy gitdomain.MergeStrategy
	Kind          domain.WorkspaceKind
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
	if c.ID == "" || c.ProjectID == "" {
		return fmt.Errorf("create workspace: missing ids: %w", asynxModels.ErrValidation)
	}
	if c.Kind != domain.WorkspaceKindHome && c.RepoID == "" {
		return fmt.Errorf("create workspace: missing repoId for git workspace: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateWorkspace) EmitEvent(
	_ *domain.Workspace,
) domain.Workspace {
	strategy := c.MergeStrategy
	if strategy == "" {
		strategy = gitdomain.MergeStrategyMerge
	}
	kind := c.Kind
	if kind == "" {
		kind = domain.WorkspaceKindGit
	}
	// Seed the lifecycle status from the protected flag: a protected branch
	// starts locked, every other workspace starts new (00 §6.1).
	status := domain.WorkspaceStatusNew
	if c.Protected {
		status = domain.WorkspaceStatusLocked
	}
	return domain.Workspace{
		ID:            c.ID,
		RepoID:        c.RepoID,
		ProjectID:     c.ProjectID,
		Branch:        c.Branch,
		WorktreePath:  c.WorktreePath,
		ForkPointSha:  c.ForkPointSha,
		ParentID:      c.ParentID,
		Status:        status,
		MergeStrategy: strategy,
		IsDefault:     c.IsDefault,
		Kind:          kind,
		LastActivity:  c.Now,
		CreatedAt:     c.Now,
	}
}
