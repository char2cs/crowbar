package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
	if c.HasPR {
		ws.PRUrl = c.PRUrl
		ws.PRTitle = c.PRTitle
		ws.PRTargetBranch = c.PRTargetBranch
	}
	ws.Status = nextProviderStatus(
		ws.Status,
		ws.LockOverride,
		c.Protected,
		c.HasPR,
		c.PRStatus,
	)
	return ws
}

func nextProviderStatus(
	current domain.WorkspaceStatus,
	lockOverride *bool,
	protected bool,
	hasPR bool,
	prStatus string,
) domain.WorkspaceStatus {
	if current == domain.WorkspaceStatusDeleted ||
		current == domain.WorkspaceStatusPRConflicts {
		return current
	}
	if lockOverride != nil {
		return nextLockStatus(current, lockOverride, protected)
	}
	if protected {
		return domain.WorkspaceStatusLocked
	}
	if current == domain.WorkspaceStatusLocked {
		return current
	}
	if hasPR {
		return prStatusToWorkspace(prStatus)
	}
	return current
}

func nextLockStatus(
	current domain.WorkspaceStatus,
	lockOverride *bool,
	protected bool,
) domain.WorkspaceStatus {
	if current == domain.WorkspaceStatusDeleted {
		return current
	}
	locked := protected
	if lockOverride != nil {
		locked = *lockOverride
	}
	if locked {
		return domain.WorkspaceStatusLocked
	}
	if current == domain.WorkspaceStatusLocked {
		return domain.WorkspaceStatusNew
	}
	return current
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
