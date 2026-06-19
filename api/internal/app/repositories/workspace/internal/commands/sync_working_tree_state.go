package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SyncWorkingTreeState recomputes the diff/conflict summary from git (00 §5.3,
// §6.1). A local conflict surfaces as Status=pr-conflicts; the base status
// otherwise stays unchanged. HasConflicts and HasCommits are transient inputs
// that drive the status, not stored fields.
type SyncWorkingTreeState struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
	Now          time.Time
}

func (c SyncWorkingTreeState) AggregateID() string {
	return c.ID
}

func (c SyncWorkingTreeState) EventName() string {
	return "workspace.working_tree_synced." + c.ID
}

func (c SyncWorkingTreeState) ShouldSnapshot() bool {
	return true
}

func (c SyncWorkingTreeState) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("sync working tree: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SyncWorkingTreeState) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Added = clampZero(c.Added)
	ws.Deleted = clampZero(c.Deleted)
	ws.LastActivity = c.Now
	// A successful mutating command clears any stale background error (00 §4).
	ws.LastError = ""
	// When local conflicts appear, surface them via the Status enum. Status
	// stays at its current value otherwise.
	if c.HasConflicts {
		ws.Status = domain.WorkspaceStatusPRConflicts
	}
	return ws
}

func clampZero(
	n int,
) int {
	if n < 0 {
		return 0
	}
	return n
}
