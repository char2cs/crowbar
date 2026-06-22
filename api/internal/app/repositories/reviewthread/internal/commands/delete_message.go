package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeleteReviewMessage removes a reply from a thread. The root comment
// (Messages[0]) cannot be removed this way — a thread must keep its root, so
// deleting the root is expressed by forgetting the whole thread instead (09 §3).
type DeleteReviewMessage struct {
	ID        string
	MessageID string
}

func (c DeleteReviewMessage) AggregateID() string {
	return c.ID
}

func (c DeleteReviewMessage) EventName() string {
	return "review_thread.message_deleted." + c.ID
}

func (c DeleteReviewMessage) ShouldSnapshot() bool {
	return false
}

func (c DeleteReviewMessage) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("delete review message: %w", asynxModels.ErrValidation)
	}
	if c.MessageID == "" {
		return fmt.Errorf("delete review message: missing message id: %w", asynxModels.ErrValidation)
	}
	if len(current.Messages) > 0 && current.Messages[0].ID == c.MessageID {
		return fmt.Errorf("delete review message: cannot delete root comment: %w", asynxModels.ErrValidation)
	}
	if !hasMessage(current.Messages, c.MessageID) {
		return fmt.Errorf("delete review message: message not found: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c DeleteReviewMessage) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	messages := make([]domain.ReviewMessage, 0, len(current.Messages))
	for _, m := range current.Messages {
		if m.ID == c.MessageID {
			continue
		}
		messages = append(messages, m)
	}
	thread.Messages = messages
	return thread
}
