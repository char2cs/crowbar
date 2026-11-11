package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetType rewrites which kind of row a chat is, leaving its id, workspace and
// conversation as they stand. Only chat and workflow are valid targets; a folder
// is refused in both directions because folders delete by promoting, not cascading.
type SetType struct {
	ID   string
	Type domain.ChatType
}

func (c SetType) AggregateID() string {
	return c.ID
}

func (c SetType) EventName() string {
	return "agentchat.type_set." + c.ID
}

func (c SetType) ShouldSnapshot() bool {
	return false
}

func (c SetType) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("set type: no chat: %w", asynxModels.ErrValidation)
	}
	if !validChatType(c.Type) {
		return fmt.Errorf("set type: invalid type: %w", asynxModels.ErrValidation)
	}
	if c.Type == domain.ChatTypeFolder || current.Type == domain.ChatTypeFolder {
		return fmt.Errorf("set type: across the folder boundary: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetType) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Type = c.Type
	return chat
}
