package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CompleteAgentRun advances a running AgentRun to done.
type CompleteAgentRun struct {
	ID string
}

func (c CompleteAgentRun) AggregateID() string {
	return c.ID
}

func (c CompleteAgentRun) EventName() string {
	return "agent_run.done." + c.ID
}

func (c CompleteAgentRun) ShouldSnapshot() bool {
	return false
}

func (c CompleteAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return fmt.Errorf("complete agent run: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.AgentRunStatusRunning {
		return fmt.Errorf("complete agent run: not running: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CompleteAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	run := *current
	run.Status = domain.AgentRunStatusDone
	return run
}
