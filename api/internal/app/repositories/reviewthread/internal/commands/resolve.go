package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResolveReviewThread marks an open thread resolved.
type ResolveReviewThread struct {
	ID string
}

func (c ResolveReviewThread) AggregateID() string {
	return c.ID
}

func (c ResolveReviewThread) EventName() string {
	return "review_thread.resolved." + c.ID
}

func (c ResolveReviewThread) ShouldSnapshot() bool {
	return false
}

func (c ResolveReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("resolve review thread: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.ReviewThreadStatusOpen {
		return fmt.Errorf("resolve review thread: not open: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ResolveReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	thread.Status = domain.ReviewThreadStatusResolved
	return thread
}
