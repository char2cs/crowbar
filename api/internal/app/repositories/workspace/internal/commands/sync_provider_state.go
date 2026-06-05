package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SyncProviderState applies a provider poll result: PR status + protected flag
// (08 §5). It only ever writes pr-* statuses (never touches "new"; you cannot
// have a PR without commits) and `locked` (00 §6.1).
type SyncProviderState struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
	Now            time.Time
}

func (c SyncProviderState) AggregateID() string {
	return c.ID
}

func (c SyncProviderState) EventName() string {
	return "workspace.provider_synced." + c.ID
}

func (c SyncProviderState) ShouldSnapshot() bool {
	return true
}

func (c SyncProviderState) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("sync provider state: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SyncProviderState) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Locked = c.Protected
	if !c.HasPR {
		return ws
	}
	ws.Status = prStatusToWorkspace(c.PRStatus)
	ws.PRUrl = c.PRUrl
	ws.PRTitle = c.PRTitle
	ws.PRTargetBranch = c.PRTargetBranch
	return ws
}

func prStatusToWorkspace(
	status string,
) domain.WorkspaceStatus {
	switch status {
	case "open":
		return domain.WorkspaceStatusPROpen
	case "merged":
		return domain.WorkspaceStatusPRMerged
	case "closed":
		return domain.WorkspaceStatusPRClosed
	default:
		return domain.WorkspaceStatusPROpen
	}
}
