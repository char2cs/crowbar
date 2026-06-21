package reviewthread

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc is an alias for the store-layer broadcast type, exposed so the
// repositories container can wire it without importing the internal store package.
type BroadcastFunc = store.BroadcastFunc

// OpenInput carries the anchor + first message for a new thread (09 §3).
type OpenInput struct {
	ID         string
	WsID       string
	FilePath   string
	LineNumber int
	StartLine  int
	EndLine    int
	Side       domain.ReviewSide
	MessageID  string
	Author     string
	IsAgent    bool
	Body       string
}

// ReviewThread is the review-thread aggregate repository.
type ReviewThread interface {
	Open(
		ctx context.Context,
		in OpenInput,
		now time.Time,
	) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		id string,
		messageID string,
		body string,
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
	List(
		ctx context.Context,
	) ([]domain.ReviewThread, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.ReviewThread, error)
}

type reviewThread struct {
	ax    asynx.Asynx[domain.ReviewThread]
	store store.Store
}

// New builds a ReviewThread repository over the given asynx instance and a GORM DB.
// The broadcast func is the hub fan-out for projected rows (03 §2).
func New(
	ax asynx.Asynx[domain.ReviewThread],
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (ReviewThread, error) {
	st, err := store.New(db, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: store: %w", err)
	}
	return &reviewThread{ax: ax, store: st}, nil
}

func (r *reviewThread) Open(
	ctx context.Context,
	in OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.OpenReviewThread{
		ID:         in.ID,
		WsID:       in.WsID,
		FilePath:   in.FilePath,
		LineNumber: in.LineNumber,
		StartLine:  in.StartLine,
		EndLine:    in.EndLine,
		Side:       in.Side,
		MessageID:  in.MessageID,
		Author:     in.Author,
		IsAgent:    in.IsAgent,
		Body:       in.Body,
		Now:        now,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: open: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Reply(
	ctx context.Context,
	id string,
	messageID string,
	body string,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.ReplyReviewThread{
		ID:        id,
		MessageID: messageID,
		Body:      body,
		Now:       now,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: reply: %w", err)
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

func (r *reviewThread) List(
	ctx context.Context,
) ([]domain.ReviewThread, error) {
	rows, err := r.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: list: %w", err)
	}
	return rows, nil
}

func (r *reviewThread) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.ReviewThread, error) {
	rows, err := r.store.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: list by workspace: %w", err)
	}
	return rows, nil
}
