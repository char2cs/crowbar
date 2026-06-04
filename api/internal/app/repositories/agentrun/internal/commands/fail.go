package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// FailAgentRun advances a running AgentRun to error. Issued by crash recovery.
type FailAgentRun struct {
	ID string
}

func (c FailAgentRun) AggregateID() string {
	return c.ID
}

func (c FailAgentRun) EventName() string {
	return "agent_run.error." + c.ID
}

func (c FailAgentRun) ShouldSnapshot() bool {
	return false
}

func (c FailAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return fmt.Errorf("fail agent run: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.AgentRunStatusRunning {
		return fmt.Errorf("fail agent run: not running: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c FailAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	run := *current
	run.Status = domain.AgentRunStatusError
	return run
}
