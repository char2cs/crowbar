package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type CreateAgentRun struct {
	ID        string
	TaskID    string
	StateName string
	Token     string
}

func (c CreateAgentRun) AggregateID() string  { return c.ID }
func (c CreateAgentRun) EventName() string    { return "agent_run.created" }
func (c CreateAgentRun) ShouldSnapshot() bool { return false }

func (c CreateAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	if c.TaskID == "" || c.StateName == "" || c.Token == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c CreateAgentRun) EmitEvent(
	_ *domain.AgentRun,
) domain.AgentRun {
	now := time.Now()
	return domain.AgentRun{
		ID:        c.ID,
		TaskID:    c.TaskID,
		StateName: c.StateName,
		Token:     c.Token,
		Status:    domain.AgentRunStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
