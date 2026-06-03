package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ResumeTask struct {
	ID string
}

func (c ResumeTask) AggregateID() string  { return c.ID }
func (c ResumeTask) EventName() string    { return "task.resumed" }
func (c ResumeTask) ShouldSnapshot() bool { return false }

func (c ResumeTask) Validate(
	current *domain.Task,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c ResumeTask) EmitEvent(
	current *domain.Task,
) domain.Task {
	t := *current
	t.Status = domain.TaskStatusRunning
	t.UpdatedAt = time.Now()
	return t
}
