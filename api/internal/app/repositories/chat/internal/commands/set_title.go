package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type SetTitle struct {
	ChatID string
	Title  string
	Source string
}

func (c SetTitle) AggregateID() string  { return c.ChatID }
func (c SetTitle) EventName() string    { return "agentchat.title_set." + c.ChatID }
func (c SetTitle) ShouldSnapshot() bool { return false }

func (c SetTitle) Validate(current *domain.Chat) error {
	if current == nil {
		return fmt.Errorf("set title: no chat: %w", asynxModels.ErrValidation)
	}
	if current.TitleLocked && c.Source != "user" {
		return fmt.Errorf("set title: title locked, non-user source rejected: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetTitle) EmitEvent(current *domain.Chat) domain.Chat {
	next := *current
	next.Title = c.Title
	if c.Source == "user" {
		next.TitleLocked = true
	}
	return next
}
