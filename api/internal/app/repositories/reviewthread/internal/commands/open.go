package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type OpenReviewThread struct {
	ID         string
	TaskID     string
	AgentRunID *string
	File       string
	Line       int
	Phase      domain.ReviewPhase
	OpenedBy   string
	FirstMsg   domain.ReviewMessage
}

func (c OpenReviewThread) AggregateID() string  { return c.ID }
func (c OpenReviewThread) EventName() string    { return "review_thread.opened" }
func (c OpenReviewThread) ShouldSnapshot() bool { return false }

func (c OpenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	if c.TaskID == "" || c.File == "" || c.OpenedBy == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c OpenReviewThread) EmitEvent(
	_ *domain.ReviewThread,
) domain.ReviewThread {
	now := time.Now()
	return domain.ReviewThread{
		ID:         c.ID,
		TaskID:     c.TaskID,
		AgentRunID: c.AgentRunID,
		File:       c.File,
		Line:       c.Line,
		Phase:      c.Phase,
		OpenedBy:   c.OpenedBy,
		Status:     domain.ReviewThreadStatusOpen,
		Messages:   []domain.ReviewMessage{c.FirstMsg},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
