package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type PauseTask struct {
	ID string
}

func (c PauseTask) AggregateID() string  { return c.ID }
func (c PauseTask) EventName() string    { return "task.paused" }
func (c PauseTask) ShouldSnapshot() bool { return false }

func (c PauseTask) Validate(
	current *domain.Task,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c PauseTask) EmitEvent(
	current *domain.Task,
) domain.Task {
	t := *current
	t.Status = domain.TaskStatusPaused
	t.UpdatedAt = time.Now()
	return t
}
