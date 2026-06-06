package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ForkChat creates a child Chat copying the parent as it currently stands, with
// parentId set (01 §4). v0 forks from the current tip; no fromTurnId.
type ForkChat struct {
	ID       string
	WsID     string
	ParentID string
	Title    string
	Now      time.Time
}

func (c ForkChat) AggregateID() string {
	return c.ID
}

func (c ForkChat) EventName() string {
	return "chat.forked." + c.ID
}

func (c ForkChat) ShouldSnapshot() bool {
	return false
}

func (c ForkChat) Validate(
	current *domain.Chat,
) error {
	if current != nil {
		return fmt.Errorf("fork chat: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" || c.ParentID == "" {
		return fmt.Errorf("fork chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ForkChat) EmitEvent(
	_ *domain.Chat,
) domain.Chat {
	return domain.Chat{
		ID:        c.ID,
		WsID:      c.WsID,
		ParentID:  c.ParentID,
		Title:     c.Title,
		Status:    domain.ChatStatusIdle,
		Type:      domain.ChatTypeChat,
		CreatedAt: c.Now,
	}
}
