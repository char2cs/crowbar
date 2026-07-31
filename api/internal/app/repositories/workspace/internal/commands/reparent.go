package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Reparent re-points a workspace at a new parent and records the new fork point
// (07 §4). The leaf-guard lives in the usecase; the command only mutates fields.
type Reparent struct {
	ID           string
	ParentID     string
	ForkPointSha string
	Now          time.Time
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
	if c.ParentID == "" || c.ForkPointSha == "" {
		return fmt.Errorf("reparent: missing parent or fork point: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Reparent) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ParentID = c.ParentID
	ws.ForkPointSha = c.ForkPointSha
	ws.LastActivity = c.Now
	// A reparent gives the workspace a fork parent, and a workspace with a fork
	// parent renders UNDER that parent — it inherits its folder from its fork
	// ancestor. Leaving a stale FolderID behind would be a row claiming two
	// places at once, which is exactly the fork-chain split the folder guards
	// refuse. Validate requires a non-empty ParentID, so this always applies.
	ws.FolderID = ""
	// A reparent always lands the branch on a clean worktree (integrated, or
	// moved-with-a-predicted-conflict — never a stuck rebase). Drop any stale
	// pr-conflicts status from a previous attempt back to a non-conflict base,
	// preserving the PR badge when there is a PR; a later provider sweep refines
	// the exact PR state. pr-conflicts is otherwise sticky (nextProviderStatus
	// never overwrites it), so this is the only place a reparent can clear it.
	if ws.Status == domain.WorkspaceStatusPRConflicts {
		ws.Status = domain.WorkspaceStatusNew
		if ws.PRUrl != "" {
			ws.Status = domain.WorkspaceStatusPROpen
		}
	}
	return ws
}
