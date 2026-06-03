package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ForceTransition struct {
	ID      string
	ToState string
}

func (c ForceTransition) AggregateID() string  { return c.ID }
func (c ForceTransition) EventName() string    { return "task.force_transitioned." + c.ToState }
func (c ForceTransition) ShouldSnapshot() bool { return false }

func (c ForceTransition) Validate(
	current *domain.Task,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	if c.ToState == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c ForceTransition) EmitEvent(
	current *domain.Task,
) domain.Task {
	t := *current
	t.CurrentState = c.ToState
	t.UpdatedAt = time.Now()
	return t
}
