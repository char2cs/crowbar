package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SetPendingMerge records a conflicted merge-into-parent awaiting resolution
// (07 §3.1, 04 §6.1).
type SetPendingMerge struct {
	ID             string
	Strategy       gitdomain.MergeStrategy
	TargetParentID string
}

func (c SetPendingMerge) AggregateID() string {
	return c.ID
}

func (c SetPendingMerge) EventName() string {
	return "workspace.pending_merge_set." + c.ID
}

func (c SetPendingMerge) ShouldSnapshot() bool {
	return false
}

func (c SetPendingMerge) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set pending merge: %w", asynxModels.ErrValidation)
	}
	if c.TargetParentID == "" {
		return fmt.Errorf("set pending merge: missing target: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetPendingMerge) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.PendingMerge = &gitdomain.PendingMerge{
		Strategy:       c.Strategy,
		TargetParentID: c.TargetParentID,
	}
	return ws
}
