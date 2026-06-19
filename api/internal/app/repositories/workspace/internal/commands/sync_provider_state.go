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
	if c.HasPR {
		ws.PRUrl = c.PRUrl
		ws.PRTitle = c.PRTitle
		ws.PRTargetBranch = c.PRTargetBranch
	}
	ws.Status = nextProviderStatus(
		ws.Status,
		c.Protected,
		c.HasPR,
		c.PRStatus,
	)
	return ws
}

// nextProviderStatus applies the D4 precedence rules
// (deleted > locked > pr-conflicts > pr-* > new):
//   - deleted/pr-conflicts are never clobbered by a provider sync;
//   - Protected → locked (wins over any incoming pr-* status);
//   - an existing locked status is preserved unless Protected was cleared;
//   - otherwise an open/merged/closed PR maps to the matching pr-* status.
func nextProviderStatus(
	current domain.WorkspaceStatus,
	protected bool,
	hasPR bool,
	prStatus string,
) domain.WorkspaceStatus {
	if current == domain.WorkspaceStatusDeleted ||
		current == domain.WorkspaceStatusPRConflicts {
		return current
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
