package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type InterruptAgentRun struct {
	ID string
}

func (c InterruptAgentRun) AggregateID() string  { return c.ID }
func (c InterruptAgentRun) EventName() string    { return "agent_run.interrupted" }
func (c InterruptAgentRun) ShouldSnapshot() bool { return false }

func (c InterruptAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c InterruptAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	r := *current
	r.Status = domain.AgentRunStatusInterrupted
	r.UpdatedAt = time.Now()
	return r
}
