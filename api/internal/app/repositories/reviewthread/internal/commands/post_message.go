package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type PostReviewThreadMessage struct {
	ThreadID string
	Message  domain.ReviewMessage
}

func (c PostReviewThreadMessage) AggregateID() string  { return c.ThreadID }
func (c PostReviewThreadMessage) EventName() string    { return "review_thread.message_posted" }
func (c PostReviewThreadMessage) ShouldSnapshot() bool { return false }

func (c PostReviewThreadMessage) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	if c.Message.Role == "" || c.Message.Content == "" {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c PostReviewThreadMessage) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	t := *current
	t.Messages = append(t.Messages, c.Message)
	t.UpdatedAt = time.Now()
	return t
}
