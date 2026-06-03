package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type CompleteAgentRun struct {
	ID     string
	Output string
}

func (c CompleteAgentRun) AggregateID() string  { return c.ID }
func (c CompleteAgentRun) EventName() string    { return "agent_run.completed" }
func (c CompleteAgentRun) ShouldSnapshot() bool { return false }

func (c CompleteAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c CompleteAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	r := *current
	now := time.Now()
	r.Status = domain.AgentRunStatusCompleted
	r.Outputs = append(r.Outputs, domain.AgentRunOutput{
		StateName: r.StateName,
		Output:    c.Output,
	})
	r.UpdatedAt = now
	return r
}
