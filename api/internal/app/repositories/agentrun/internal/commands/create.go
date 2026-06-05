package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateAgentRun creates an AgentRun aggregate in the pending state.
type CreateAgentRun struct {
	ID     string
	WsID   string
	ChatID string
	Now    time.Time
}

func (c CreateAgentRun) AggregateID() string {
	return c.ID
}

func (c CreateAgentRun) EventName() string {
	return "agent_run.created." + c.ID
}

func (c CreateAgentRun) ShouldSnapshot() bool {
	return false
}

func (c CreateAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current != nil {
		return fmt.Errorf("create agent run: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" || c.ChatID == "" {
		return fmt.Errorf("create agent run: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateAgentRun) EmitEvent(
	_ *domain.AgentRun,
) domain.AgentRun {
	return domain.AgentRun{
		ID:        c.ID,
		WsID:      c.WsID,
		ChatID:    c.ChatID,
		Status:    domain.AgentRunStatusPending,
		CreatedAt: c.Now,
	}
}
