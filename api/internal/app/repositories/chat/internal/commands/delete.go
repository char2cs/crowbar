package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeleteChat soft-deletes a chat (sets deletedAt). Idempotent: re-issuing on an
// already-deleted chat is a no-op, so cascades replay safely (01 §8).
type DeleteChat struct {
	ID  string
	Now time.Time
}

func (c DeleteChat) AggregateID() string {
	return c.ID
}

func (c DeleteChat) EventName() string {
	return "chat.deleted." + c.ID
}

func (c DeleteChat) ShouldSnapshot() bool {
	return false
}

func (c DeleteChat) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("delete chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c DeleteChat) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	if chat.DeletedAt != nil {
		return chat
	}
	deletedAt := c.Now
	chat.DeletedAt = &deletedAt
	return chat
}
