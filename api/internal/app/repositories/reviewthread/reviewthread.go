package reviewthread

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
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
		author string,
		isAgent bool,
		body string,
		now time.Time,
	) (domain.ReviewThread, error)
	EditMessage(
		ctx context.Context,
		id string,
		messageID string,
		body string,
	) (domain.ReviewThread, error)
	DeleteMessage(
		ctx context.Context,
		id string,
		messageID string,
	) (domain.ReviewThread, error)
	DeleteThread(
		ctx context.Context,
		id string,
	) error
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
	ax      asynx.Asynx[domain.ReviewThread]
	store   store.Store
	writeMu serialize.KeyedMutex
}

// New builds a ReviewThread repository over the given asynx instance and a GORM DB.
// The broadcast func is the hub fan-out for projected rows (03 §2).
//
// The reviewthread aggregate is global: one asynx and one event store hold every
// thread. es is that shared event store; when it exposes serialize.AggregateLister,
// New enumerates its ids so store.New can reconcile each read-model row from the
// event log on open (heals rows a dropped projection left missing). Enumeration is
// best-effort — a failure logs and falls back to no reconcile, exactly as if no
// row needed healing.
func New(
	ctx context.Context,
	ax asynx.Asynx[domain.ReviewThread],
	es asynxModels.Store,
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (ReviewThread, error) {
	ids := aggregateIDs(ctx, es)
	st, err := store.New(ctx, db, ax, broadcast, ids)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: store: %w", err)
	}
	return &reviewThread{ax: ax, store: st}, nil
}

// aggregateIDs enumerates the event store's aggregate ids when it supports the
// optional AggregateLister capability, so the read model can reconcile every
// thread on open. A store without the capability, or an enumeration error, yields
// no ids (reconcile is then a no-op).
func aggregateIDs(
	ctx context.Context,
	es asynxModels.Store,
) []string {
	lister, ok := es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	ids, err := lister.AggregateIDs(ctx)
	if err != nil {
		slog.WarnContext(ctx, "reviewthread: enumerate aggregate ids for reconcile failed; skipping reconcile", "err", err)
		return nil
	}
	return ids
}

func (r *reviewThread) Open(
	ctx context.Context,
	in OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	r.writeMu.Lock(in.ID)
	defer r.writeMu.Unlock(in.ID)
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
	author string,
	isAgent bool,
	body string,
	now time.Time,
) (domain.ReviewThread, error) {
	r.writeMu.Lock(id)
	defer r.writeMu.Unlock(id)
	evt, err := r.ax.SendWait(ctx, commands.ReplyReviewThread{
		ID:        id,
		MessageID: messageID,
		Author:    author,
		IsAgent:   isAgent,
		Body:      body,
		Now:       now,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: reply: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) EditMessage(
	ctx context.Context,
	id string,
	messageID string,
	body string,
) (domain.ReviewThread, error) {
	r.writeMu.Lock(id)
	defer r.writeMu.Unlock(id)
	evt, err := r.ax.SendWait(ctx, commands.EditReviewMessage{
		ID:        id,
		MessageID: messageID,
		Body:      body,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: edit message: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) DeleteMessage(
	ctx context.Context,
	id string,
	messageID string,
) (domain.ReviewThread, error) {
	r.writeMu.Lock(id)
	defer r.writeMu.Unlock(id)
	evt, err := r.ax.SendWait(ctx, commands.DeleteReviewMessage{
		ID:        id,
		MessageID: messageID,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: delete message: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) DeleteThread(
	ctx context.Context,
	id string,
) error {
	r.writeMu.Lock(id)
	defer r.writeMu.Unlock(id)
	if err := r.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("reviewthread: delete thread: %w", err)
	}
	return nil
}

func (r *reviewThread) Resolve(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	r.writeMu.Lock(id)
	defer r.writeMu.Unlock(id)
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
	r.writeMu.Lock(id)
	defer r.writeMu.Unlock(id)
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
