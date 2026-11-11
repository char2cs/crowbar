package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Reparent writes a new fork parent and fork point: git lineage, distinct from a
// sidebar drag, which moves only the chat's tree placement. The leaf guard lives
// in the usecase; the command only mutates fields.
type Reparent struct {
	ID              string
	NewForkParentID string
	ForkPointSha    string
	Now             time.Time
}

func (c Reparent) AggregateID() string {
	return c.ID
}

func (c Reparent) EventName() string {
	return "workspace.reparented." + c.ID
}

func (c Reparent) ShouldSnapshot() bool {
	return true
}

func (c Reparent) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("reparent: %w", asynxModels.ErrValidation)
	}
	// No parent is the top of a lineage (a kept child whose parent was deleted).
	if c.NewForkParentID != "" && c.ForkPointSha == "" {
		return fmt.Errorf("reparent: missing fork point: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Reparent) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ParentID = c.NewForkParentID
	ws.ForkPointSha = c.ForkPointSha
	ws.LastActivity = c.Now
	// A reparent always lands on a clean worktree, so a stale pr-conflicts (sticky
	// against provider syncs) drops back to a non-conflict base, keeping the PR badge.
	if ws.Status == domain.WorkspaceStatusPRConflicts {
		ws.Status = domain.WorkspaceStatusNew
		if ws.PRUrl != "" {
			ws.Status = domain.WorkspaceStatusPROpen
		}
	}
	return ws
}
