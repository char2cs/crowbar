package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetPlacement writes where a Node sits in the tree: the row it hangs off and
// its dense index within that sibling space — the write the ONE row actually
// dragged owes, mirrors agentchat's SetPlacement
// (internal/app/repositories/chat/internal/commands/set_placement.go).
//
// The command mutates fields and nothing else. The golden-rule containment
// checks (2026-09-08 sidebar-placement-unification design §2.6 — a repo may
// only file under project home, a workspace only within its own repo's tree,
// and so on) live in the usecase, which can see the whole tree; a command sees
// one aggregate and could not check them.
type SetPlacement struct {
	ID       string
	ParentID string
	Order    int
}

func (c SetPlacement) AggregateID() string  { return c.ID }
func (c SetPlacement) EventName() string    { return "node.placement_set." + c.ID }
func (c SetPlacement) ShouldSnapshot() bool { return false }

func (c SetPlacement) Validate(current *domain.Node) error {
	if current == nil {
		return fmt.Errorf("set placement: no node: %w", asynxModels.ErrValidation)
	}
	if c.ParentID == c.ID {
		return fmt.Errorf("set placement: node under itself: %w", asynxModels.ErrValidation)
	}
	if c.Order < 0 {
		return fmt.Errorf("set placement: negative order: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetPlacement) EmitEvent(current *domain.Node) domain.Node {
	n := *current
	n.ParentID = c.ParentID
	n.Order = c.Order
	return n
}
