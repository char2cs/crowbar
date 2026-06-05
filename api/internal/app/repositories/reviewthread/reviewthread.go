package reviewthread

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReviewThread is the review-thread aggregate repository.
type ReviewThread interface {
	Open(
		ctx context.Context,
		id string,
		wsID string,
		now time.Time,
	) (domain.ReviewThread, error)
	Resolve(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	Reopen(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
}

type reviewThread struct {
	ax asynx.Asynx[domain.ReviewThread]
}

// New builds a ReviewThread repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.ReviewThread],
) ReviewThread {
	return &reviewThread{ax: ax}
}

func (r *reviewThread) Open(
	ctx context.Context,
	id string,
	wsID string,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.OpenReviewThread{ID: id, WsID: wsID, Now: now})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: open: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Resolve(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.ResolveReviewThread{ID: id})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: resolve: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Reopen(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.ReopenReviewThread{ID: id})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: reopen: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Get(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	got, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: get: %w", err)
	}
	return got, nil
}
