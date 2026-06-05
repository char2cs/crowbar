package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReplyReviewThread appends a message to an existing thread (09 §3).
type ReplyReviewThread struct {
	ID        string
	MessageID string
	Author    string
	IsAgent   bool
	Body      string
	Now       time.Time
}

func (c ReplyReviewThread) AggregateID() string {
	return c.ID
}

func (c ReplyReviewThread) EventName() string {
	return "review_thread.replied." + c.ID
}

func (c ReplyReviewThread) ShouldSnapshot() bool {
	return false
}

func (c ReplyReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("reply review thread: %w", asynxModels.ErrValidation)
	}
	if c.MessageID == "" {
		return fmt.Errorf("reply review thread: missing message id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ReplyReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	thread.Messages = append(thread.Messages, domain.ReviewMessage{
		ID:        c.MessageID,
		Author:    c.Author,
		IsAgent:   c.IsAgent,
		Body:      c.Body,
		CreatedAt: c.Now,
	})
	return thread
}
