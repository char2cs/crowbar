package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateChat creates a root Chat aggregate in the idle state (01 §3).
type CreateChat struct {
	ID    string
	WsID  string
	Title string
	Now   time.Time
}

func (c CreateChat) AggregateID() string {
	return c.ID
}

func (c CreateChat) EventName() string {
	return "chat.created." + c.ID
}

func (c CreateChat) ShouldSnapshot() bool {
	return false
}

func (c CreateChat) Validate(
	current *domain.Chat,
) error {
	if current != nil {
		return fmt.Errorf("create chat: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" {
		return fmt.Errorf("create chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateChat) EmitEvent(
	_ *domain.Chat,
) domain.Chat {
	return domain.Chat{
		ID:        c.ID,
		WsID:      c.WsID,
		Title:     c.Title,
		Status:    domain.ChatStatusIdle,
		Type:      domain.ChatTypeChat,
		CreatedAt: c.Now,
	}
}
