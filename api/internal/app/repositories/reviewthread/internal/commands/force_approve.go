package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ForceApproveReviewThread struct {
	ThreadID string
}

func (c ForceApproveReviewThread) AggregateID() string  { return c.ThreadID }
func (c ForceApproveReviewThread) EventName() string    { return "review_thread.force_approved" }
func (c ForceApproveReviewThread) ShouldSnapshot() bool { return false }

func (c ForceApproveReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c ForceApproveReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	t := *current
	t.Status = domain.ReviewThreadStatusForceApproved
	t.UpdatedAt = time.Now()
	return t
}
