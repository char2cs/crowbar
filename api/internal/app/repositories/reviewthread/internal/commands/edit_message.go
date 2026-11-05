package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// EditReviewMessage rewrites the body of any message in a thread by id, the root
// comment (Messages[0]) included. Edits are not timestamped.
type EditReviewMessage struct {
	ID        string
	MessageID string
	Body      string
}

func (c EditReviewMessage) AggregateID() string {
	return c.ID
}

func (c EditReviewMessage) EventName() string {
	return "review_thread.message_edited." + c.ID
}

func (c EditReviewMessage) ShouldSnapshot() bool {
	return false
}

func (c EditReviewMessage) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return domain.ReviewThreadNotFound(asynxModels.ErrValidation)
	}
	if c.MessageID == "" {
		return fmt.Errorf("edit review message: missing message id: %w", asynxModels.ErrValidation)
	}
	if c.Body == "" {
		return fmt.Errorf("edit review message: empty body: %w", asynxModels.ErrValidation)
	}
	if !hasMessage(current.Messages, c.MessageID) {
		return fmt.Errorf("edit review message: message not found: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c EditReviewMessage) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	messages := make([]domain.ReviewMessage, len(current.Messages))
	copy(messages, current.Messages)
	for i := range messages {
		if messages[i].ID == c.MessageID {
			messages[i].Body = c.Body
			break
		}
	}
	thread.Messages = messages
	return thread
}

// hasMessage reports whether a message with the given id exists in the slice.
func hasMessage(
	messages []domain.ReviewMessage,
	id string,
) bool {
	for _, m := range messages {
		if m.ID == id {
			return true
		}
	}
	return false
}
