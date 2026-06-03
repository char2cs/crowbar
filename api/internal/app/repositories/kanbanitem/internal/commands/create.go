package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type CreateKanbanItem struct {
	ID         string
	TaskID     string
	Title      string
	AgentRunID string
}

func (c CreateKanbanItem) AggregateID() string  { return c.ID }
func (c CreateKanbanItem) EventName() string    { return "kanban_item.created" }
func (c CreateKanbanItem) ShouldSnapshot() bool { return false }

func (c CreateKanbanItem) Validate(
	current *domain.KanbanItem,
) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	if c.TaskID == "" || c.Title == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c CreateKanbanItem) EmitEvent(
	_ *domain.KanbanItem,
) domain.KanbanItem {
	now := time.Now()
	return domain.KanbanItem{
		ID:         c.ID,
		TaskID:     c.TaskID,
		Title:      c.Title,
		Status:     "open",
		AgentRunID: c.AgentRunID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
