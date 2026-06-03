package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type RetryTask struct {
	ID string
}

func (c RetryTask) AggregateID() string  { return c.ID }
func (c RetryTask) EventName() string    { return "task.retried" }
func (c RetryTask) ShouldSnapshot() bool { return false }

func (c RetryTask) Validate(
	current *domain.Task,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c RetryTask) EmitEvent(
	current *domain.Task,
) domain.Task {
	t := *current
	t.Status = domain.TaskStatusRunning
	t.UpdatedAt = time.Now()
	return t
}
