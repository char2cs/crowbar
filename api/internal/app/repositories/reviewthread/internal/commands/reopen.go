package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReopenReviewThread re-opens a resolved thread.
type ReopenReviewThread struct {
	ID string
}

func (c ReopenReviewThread) AggregateID() string {
	return c.ID
}

func (c ReopenReviewThread) EventName() string {
	return "review_thread.reopened." + c.ID
}

func (c ReopenReviewThread) ShouldSnapshot() bool {
	return false
}

func (c ReopenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("reopen review thread: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.ReviewThreadStatusResolved {
		return fmt.Errorf("reopen review thread: not resolved: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ReopenReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	thread.Status = domain.ReviewThreadStatusOpen
	return thread
}
