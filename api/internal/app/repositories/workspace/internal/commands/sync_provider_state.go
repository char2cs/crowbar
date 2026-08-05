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
	// A successful mutating command clears any stale background error (00 §4).
	ws.LastError = ""
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

// nextProviderStatus applies the D4 precedence rules
// (deleted > locked > pr-conflicts > pr-* > new):
//   - deleted/pr-conflicts are never clobbered by a provider sync;
//   - Protected → locked (wins over any incoming pr-* status);
//   - an existing locked status is preserved unless Protected was cleared;
//   - otherwise an open/merged/closed PR maps to the matching pr-* status.
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
	// The user's own lock decision outranks the provider's protected flag, in
	// BOTH directions. Without this branch the poll below would re-lock a branch
	// the user deliberately unlocked, every minute, forever — and would let an
	// incoming pr-* status quietly unlock one they deliberately locked.
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

// nextLockStatus resolves a workspace's status against a lock decision.
//
// Shared by SetLock (applying the user's choice now) and nextProviderStatus
// (defending it on every subsequent poll), because those are the same question
// asked twice and the two drifting apart is precisely how an unlocked branch
// finds itself locked again.
//
// Unlocking has to put SOMETHING in place of `locked`. It cannot leave the
// status alone — that is the very value being removed — so it falls back to the
// lifecycle status the branch would have carried had it never been protected:
// `new`, which is also what create.go seeds an ordinary branch with. The next
// provider poll refines that into a pr-* status if there is a PR.
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
