package commands

import (
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ResolveReviewThread struct {
	ThreadID string
	Emoji    string
}

func (c ResolveReviewThread) AggregateID() string  { return c.ThreadID }
func (c ResolveReviewThread) EventName() string    { return "review_thread.resolved" }
func (c ResolveReviewThread) ShouldSnapshot() bool { return false }

func (c ResolveReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return asynxModels.ErrNotFound
	}
	return nil
}

func (c ResolveReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	t := *current
	t.Status = domain.ReviewThreadStatusAgreed
	t.Emoji = c.Emoji
	t.UpdatedAt = time.Now()
	return t
}
