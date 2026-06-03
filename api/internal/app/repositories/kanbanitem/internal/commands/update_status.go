package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type UpdateKanbanItemStatus struct {
	ID         string
	Status     string
	AgentRunID string
}

func (c UpdateKanbanItemStatus) AggregateID() string  { return c.ID }
func (c UpdateKanbanItemStatus) EventName() string    { return "kanban_item.status_updated" }
func (c UpdateKanbanItemStatus) ShouldSnapshot() bool { return false }

func (c UpdateKanbanItemStatus) Validate(
	current *domain.KanbanItem,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	if c.Status == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c UpdateKanbanItemStatus) EmitEvent(
	current *domain.KanbanItem,
) domain.KanbanItem {
	item := *current
	item.Status = c.Status
	item.AgentRunID = c.AgentRunID
	item.UpdatedAt = time.Now()
	return item
}
