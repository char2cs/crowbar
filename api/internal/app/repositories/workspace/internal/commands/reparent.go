package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Reparent writes a new fork parent and fork point onto the workspace
// aggregate (07 §4). It IS a directly authored write: the live
// POST .../workspaces/:wsId/reparent route reaches it through
// hierarchy.Reparent, and RebaseOntoParent re-runs it for the same child. The
// chat-side move does NOT: tree.Move and tree.PlaceChat write
// domain.Chat.ParentID alone and never touch domain.Workspace.ParentID, so a
// sidebar drag is organisation and this command is git lineage. The
// leaf-guard lives in the usecase; the command only mutates fields.
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
	if c.NewForkParentID == "" || c.ForkPointSha == "" {
		return fmt.Errorf("reparent: missing parent or fork point: %w", asynxModels.ErrValidation)
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
