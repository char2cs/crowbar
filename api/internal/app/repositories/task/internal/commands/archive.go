package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ArchiveTask struct {
	ID string
}

func (c ArchiveTask) AggregateID() string  { return c.ID }
func (c ArchiveTask) EventName() string    { return "task.archived" }
func (c ArchiveTask) ShouldSnapshot() bool { return false }

func (c ArchiveTask) Validate(
	current *domain.Task,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c ArchiveTask) EmitEvent(
	current *domain.Task,
) domain.Task {
	t := *current
	t.Status = domain.TaskStatusArchived
	t.UpdatedAt = time.Now()
	return t
}
