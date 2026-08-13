package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetOrder writes a chat's index within the sibling space it is already in, and
// says nothing about which space that is.
//
// A drag renumbers a whole level and re-parents exactly one row. Writing the
// rest of that level through SetPlacement made every renumber restate a parent
// the caller had READ rather than decided, and callers read the asynchronous
// projection: a second drag landing inside the first one's projection window
// wrote a stale parent back, returning a just-filed thread to the panel root.
//
// The parent survives because it comes from `current`, which asynx folds from
// the event log — so it is current, not merely untouched.
type SetOrder struct {
	ID    string
	Order int
}

func (c SetOrder) AggregateID() string {
	return c.ID
}

func (c SetOrder) EventName() string {
	return "agentchat.order_set." + c.ID
}

func (c SetOrder) ShouldSnapshot() bool {
	return false
}

func (c SetOrder) Validate(
	current *domain.AgentChat,
) error {
	if current == nil {
		return fmt.Errorf("set order: no chat: %w", asynxModels.ErrValidation)
	}
	if c.Order < 0 {
		return fmt.Errorf("set order: negative order: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetOrder) EmitEvent(
	current *domain.AgentChat,
) domain.AgentChat {
	chat := *current
	chat.Order = c.Order
	return chat
}
