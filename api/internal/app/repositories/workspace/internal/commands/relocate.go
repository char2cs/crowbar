package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Relocate records that a workspace's worktree now lives somewhere else, without
// touching its branch, lineage or status.
//
// It exists for exactly one caller: the boot pass that moves workspaces from the
// old name-derived layout to the identity-keyed one. Nothing else moves a
// worktree — that is the point of keying the root by id — so this is the only
// command in the vocabulary that means "the same workspace, at a new path".
//
// Before it existed the migration would have had to reuse RenameBranch and pass
// the branch back unchanged, which reads as a rename that renames nothing and
// would have made the event log lie about what happened.
type Relocate struct {
	ID           string
	WorktreePath string
}

func (c Relocate) AggregateID() string {
	return c.ID
}

func (c Relocate) EventName() string {
	return "workspace.relocated." + c.ID
}

func (c Relocate) ShouldSnapshot() bool {
	return true
}

func (c Relocate) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("relocate: %w", asynxModels.ErrValidation)
	}
	if c.WorktreePath == "" {
		return fmt.Errorf("relocate: missing worktree path: %w", asynxModels.ErrValidation)
	}
	// A workspace with no worktree has nothing to relocate: it is either an
	// unprovisioned placeholder, whose path is set by provisioning, or already
	// torn down. Either way this command would be inventing a tree.
	if current.WorktreePath == "" {
		return fmt.Errorf("relocate: workspace has no worktree: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Relocate) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	// A successful mutating command clears any stale background error (00 §4).
	ws.LastError = ""
	ws.WorktreePath = c.WorktreePath
	return ws
}
