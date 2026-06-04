package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenReviewThread creates a ReviewThread aggregate in the open state.
type OpenReviewThread struct {
	ID   string
	WsID string
	Now  time.Time
}

func (c OpenReviewThread) AggregateID() string {
	return c.ID
}

func (c OpenReviewThread) EventName() string {
	return "review_thread.opened." + c.ID
}

func (c OpenReviewThread) ShouldSnapshot() bool {
	return false
}

func (c OpenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current != nil {
		return fmt.Errorf("open review thread: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" {
		return fmt.Errorf("open review thread: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c OpenReviewThread) EmitEvent(
	_ *domain.ReviewThread,
) domain.ReviewThread {
	return domain.ReviewThread{
		ID:        c.ID,
		WsID:      c.WsID,
		Status:    domain.ReviewThreadStatusOpen,
		CreatedAt: c.Now,
	}
}
