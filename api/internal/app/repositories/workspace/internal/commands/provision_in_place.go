package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProvisionInPlace flips a placeholder
// into a healthy managed worktree WITHOUT creating a new aggregate: it records
// the now-attached worktree path + fork point and clears HeldByPath, leaving
// Status = locked and every other field untouched (spec §3.3 Retry-in-place).
type ProvisionInPlace struct {
	ID           string
	WorktreePath string
	ForkPointSha string
}

func (c ProvisionInPlace) AggregateID() string {
	return c.ID
}

func (c ProvisionInPlace) EventName() string {
	return "workspace.provisioned_in_place." + c.ID
}

func (c ProvisionInPlace) ShouldSnapshot() bool {
	return true
}

func (c ProvisionInPlace) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("provision in place: %w", asynxModels.ErrValidation)
	}
	if c.WorktreePath == "" {
		return fmt.Errorf("provision in place: missing worktree path: %w", asynxModels.ErrValidation)
	}
	if current.Provisioning != domain.WorkspacePlaceholder {
		return fmt.Errorf("provision in place: not a placeholder: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ProvisionInPlace) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.WorktreePath = c.WorktreePath
	ws.ForkPointSha = c.ForkPointSha
	ws.HeldByPath = ""
	ws.Provisioning = domain.WorkspaceProvisioned
	return ws
}
