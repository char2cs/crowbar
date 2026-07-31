package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetPlacement writes the two SIDEBAR fields and nothing else: the folder the
// workspace is filed under and its dense index within that sibling space. It
// never touches ParentID — the fork lineage is a git fact and moves only through
// Reparent — so a drag in the sidebar can never rewrite what a branch was forked
// from. The fork-chain and cross-repo guards live in the folder usecase; the
// command only mutates fields.
type SetPlacement struct {
	ID       string
	FolderID string
	Order    int
}

func (c SetPlacement) AggregateID() string {
	return c.ID
}

func (c SetPlacement) EventName() string {
	return "workspace.placement_set." + c.ID
}

func (c SetPlacement) ShouldSnapshot() bool {
	return false
}

func (c SetPlacement) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set placement: %w", asynxModels.ErrValidation)
	}
	if c.Order < 0 {
		return fmt.Errorf("set placement: negative order: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetPlacement) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.FolderID = c.FolderID
	ws.Order = c.Order
	return ws
}
