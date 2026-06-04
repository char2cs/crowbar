package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResetChatIdle forces a Chat to idle. Idempotent: resetting an already-idle chat
// emits the same idle state without error, so concurrent crash-recovery passes
// cannot conflict (00 §6.2).
type ResetChatIdle struct {
	ID string
}

func (c ResetChatIdle) AggregateID() string {
	return c.ID
}

func (c ResetChatIdle) EventName() string {
	return "chat.idle_reset." + c.ID
}

func (c ResetChatIdle) ShouldSnapshot() bool {
	return false
}

func (c ResetChatIdle) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("reset chat idle: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ResetChatIdle) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Status = domain.ChatStatusIdle
	return chat
}
