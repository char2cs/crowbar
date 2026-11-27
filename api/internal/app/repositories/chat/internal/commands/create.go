package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type Create struct {
	ID          string
	WorkspaceID string
	RepoID      string
	Type        domain.ChatType
	Surface     string
	ProviderID  string
	Now         time.Time
}

func (c Create) AggregateID() string  { return c.ID }
func (c Create) EventName() string    { return "agentchat.created." + c.ID }
func (c Create) ShouldSnapshot() bool { return false }

func (c Create) Validate(current *domain.Chat) error {
	if current != nil {
		return &problem.Error{Code: problem.IDConflict, Message: fmt.Sprintf("chat %s already exists", c.ID)}
	}
	if c.ID == "" {
		return fmt.Errorf("create agent chat: missing ids: %w", asynxModels.ErrValidation)
	}
	if !validChatType(c.Type) {
		return fmt.Errorf("create agent chat: invalid type: %w", asynxModels.ErrValidation)
	}
	return nil
}

func validChatType(t domain.ChatType) bool {
	switch t {
	case domain.ChatTypeChat, domain.ChatTypeWorkflow:
		return true
	case domain.ChatTypeFolder, domain.ChatTypeBranch:
		return false
	default:
		return false
	}
}

func (c Create) EmitEvent(_ *domain.Chat) domain.Chat {
	return domain.Chat{
		ID:          c.ID,
		WorkspaceID: c.WorkspaceID,
		RepoID:      c.RepoID,
		Type:        c.Type,
		Surface:     c.Surface,
		ProviderID:  c.ProviderID,
		CreatedAt:   c.Now,
	}
}
