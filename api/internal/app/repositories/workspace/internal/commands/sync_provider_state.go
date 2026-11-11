package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SyncProviderState applies a provider poll result: PR status + protected flag.
// It only ever writes pr-* statuses (never touches "new"; you cannot
// have a PR without commits) and `locked`.
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

// nextProviderStatus applies the status precedence: deleted and pr-conflicts are
// never clobbered; a user lock decision, then Protected, yields locked over any
// pr-*; an existing lock is kept; otherwise a PR maps to its pr-* status.
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
	// The user's lock decision outranks the protected flag in both directions, or
	// every poll would re-lock an unlocked branch or unlock a locked one.
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

// nextLockStatus resolves a status against a lock decision, shared by SetLock and
// every poll so the two cannot drift. Unlocking falls back to `new`, the status
// an unprotected branch is created with; the next poll refines it to pr-*.
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
