package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type AdvanceState struct {
	ID        string
	NextState string
	Event     string
}

func (c AdvanceState) AggregateID() string  { return c.ID }
func (c AdvanceState) EventName() string    { return "task.state_advanced." + c.NextState }
func (c AdvanceState) ShouldSnapshot() bool { return false }

func (c AdvanceState) Validate(
	current *domain.Task,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	if c.NextState == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c AdvanceState) EmitEvent(
	current *domain.Task,
) domain.Task {
	t := *current
	t.CurrentState = c.NextState
	t.Status = domain.TaskStatusRunning
	t.UpdatedAt = time.Now()
	return t
}
