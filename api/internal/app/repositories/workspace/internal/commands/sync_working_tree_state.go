package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SyncWorkingTreeState recomputes diff/conflict summary from git and clears the
// "new" status once the branch has commits (00 §5.3, §6.1). HasCommits is a
// transient input, not a stored field.
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
	ws.HasConflicts = c.HasConflicts
	ws.LastActivity = c.Now
	if ws.Status == domain.WorkspaceStatusNew && c.HasCommits {
		ws.Status = ""
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
