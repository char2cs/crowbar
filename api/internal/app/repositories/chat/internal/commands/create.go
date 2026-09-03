package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Create seeds a new AgentChat: an identity, a workspace and a clock, and
// nothing else. It carries no segment, no provider and no terminal session
// because a chat does not own the process talking to it — the runner does, and
// it is Started as its own aggregate (agentrunner). A chat can therefore be
// minted by the reducer (a /clear that lands on an unknown conversation) without
// any process fact being invented for it.
type Create struct {
	ID          string
	WorkspaceID string
	Now         time.Time
}

func (c Create) AggregateID() string  { return c.ID }
func (c Create) EventName() string    { return "agentchat.created." + c.ID }
func (c Create) ShouldSnapshot() bool { return false }

func (c Create) Validate(current *domain.Chat) error {
	if current != nil {
		return fmt.Errorf("create agent chat: exists: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WorkspaceID == "" {
		return fmt.Errorf("create agent chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Create) EmitEvent(_ *domain.Chat) domain.Chat {
	return domain.Chat{
		ID:             c.ID,
		WorkspaceID:    c.WorkspaceID,
		CreatedAt:      c.Now,
		LastActivityAt: c.Now,
	}
}
