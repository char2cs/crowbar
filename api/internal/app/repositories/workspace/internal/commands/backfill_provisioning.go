package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BackfillProvisioning records the Provisioning of a workspace written before the
// field existed, the one place the legacy encoding is read; it is refused once set.
// Purge safety for a shared parent comes from WorktreeRemover's own-root proof.
type BackfillProvisioning struct {
	ID string
}

func (c BackfillProvisioning) AggregateID() string {
	return c.ID
}

func (c BackfillProvisioning) EventName() string {
	return "workspace.provisioning_backfilled." + c.ID
}

func (c BackfillProvisioning) ShouldSnapshot() bool {
	return true
}

func (c BackfillProvisioning) Validate(
	current *domain.Workspace,
) error {
	if current == nil || current.Provisioning != "" {
		return fmt.Errorf("backfill provisioning: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BackfillProvisioning) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	switch {
	case ws.Kind == domain.WorkspaceKindHome || ws.IsDefault:
		ws.Provisioning = domain.WorkspaceShared
	case ws.WorktreePath == "":
		ws.Provisioning = domain.WorkspacePlaceholder
	default:
		ws.Provisioning = domain.WorkspaceProvisioned
	}
	return ws
}
