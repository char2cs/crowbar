package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type SetPermissionLevel struct {
	ChatID string
	Level  string
	// Explicit is false for a mint-time seed (the chat merely INHERITS this
	// value; ChatSelection re-resolves the live global default for it on
	// every future spawn) and true for an explicit SetChatPermissionLevel
	// pick (which then pins the chat against the global dial for good).
	Explicit bool
}

func (c SetPermissionLevel) AggregateID() string  { return c.ChatID }
func (c SetPermissionLevel) EventName() string    { return "agentchat.permission_level_set." + c.ChatID }
func (c SetPermissionLevel) ShouldSnapshot() bool { return false }

func (c SetPermissionLevel) Validate(current *domain.Chat) error {
	if current == nil {
		return fmt.Errorf("set permission level: no chat: %w", asynxModels.ErrValidation)
	}
	if c.Level == "" {
		return fmt.Errorf("set permission level: missing level: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetPermissionLevel) EmitEvent(current *domain.Chat) domain.Chat {
	next := *current
	next.PermissionLevel = c.Level
	next.PermissionLevelExplicit = c.Explicit
	return next
}
