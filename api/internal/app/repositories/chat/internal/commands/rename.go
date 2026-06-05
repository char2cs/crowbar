package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameChat updates a chat's title (01 §3).
type RenameChat struct {
	ID    string
	Title string
}

func (c RenameChat) AggregateID() string {
	return c.ID
}

func (c RenameChat) EventName() string {
	return "chat.renamed." + c.ID
}

func (c RenameChat) ShouldSnapshot() bool {
	return false
}

func (c RenameChat) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("rename chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RenameChat) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Title = c.Title
	return chat
}
