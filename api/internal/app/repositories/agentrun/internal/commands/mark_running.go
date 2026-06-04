package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// MarkAgentRunRunning advances a pending AgentRun to running.
type MarkAgentRunRunning struct {
	ID string
}

func (c MarkAgentRunRunning) AggregateID() string {
	return c.ID
}

func (c MarkAgentRunRunning) EventName() string {
	return "agent_run.running." + c.ID
}

func (c MarkAgentRunRunning) ShouldSnapshot() bool {
	return false
}

func (c MarkAgentRunRunning) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return fmt.Errorf("mark running: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.AgentRunStatusPending {
		return fmt.Errorf("mark running: not pending: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkAgentRunRunning) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	run := *current
	run.Status = domain.AgentRunStatusRunning
	return run
}
