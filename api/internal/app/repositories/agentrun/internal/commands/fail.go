package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type FailAgentRun struct {
	ID string
}

func (c FailAgentRun) AggregateID() string  { return c.ID }
func (c FailAgentRun) EventName() string    { return "agent_run.failed" }
func (c FailAgentRun) ShouldSnapshot() bool { return false }

func (c FailAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c FailAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	r := *current
	r.Status = domain.AgentRunStatusFailed
	r.UpdatedAt = time.Now()
	return r
}
