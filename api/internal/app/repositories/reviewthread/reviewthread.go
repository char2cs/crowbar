package reviewthread

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type BroadcastFunc = store.BroadcastFunc

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
	ProviderID string
	ChatID     string
	Body       string
}

type ReplyInput struct {
	ID         string
	MessageID  string
	Author     string
	IsAgent    bool
	ProviderID string
	ChatID     string
	Body       string
}

type ReviewThread interface {
	Open(
		ctx context.Context,
		in OpenInput,
		now time.Time,
	) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		in ReplyInput,
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
	ax    asynx.Asynx[domain.ReviewThread]
	store store.Store
}

type ReadModel = store.Projection

func OpenReadModel(
	storeDB *gormdb.DB,
) (*ReadModel, error) {
	rm, err := store.Open(storeDB)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: read model: %w", err)
	}
	return rm, nil
}

func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.ReviewThread]) {
	return store.Subscriber(rm)
}

func New(
	ax asynx.Asynx[domain.ReviewThread],
	es asynxModels.Store,
	rm *ReadModel,
	broadcast BroadcastFunc,
) (ReviewThread, error) {
	st, err := store.New(rm, es, ax, broadcast)
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
	evt, err := r.ax.Send(ctx, commands.OpenReviewThread{
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
		ProviderID: in.ProviderID,
		ChatID:     in.ChatID,
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
	in ReplyInput,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.ax.Send(ctx, commands.ReplyReviewThread{
		ID:         in.ID,
		MessageID:  in.MessageID,
		Author:     in.Author,
		IsAgent:    in.IsAgent,
		ProviderID: in.ProviderID,
		ChatID:     in.ChatID,
		Body:       in.Body,
		Now:        now,
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
	evt, err := r.ax.Send(ctx, commands.EditReviewMessage{
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
	evt, err := r.ax.Send(ctx, commands.DeleteReviewMessage{
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
	err := r.ax.Forget(ctx, id)
	if errors.Is(err, asynxModels.ErrValidation) {
		return domain.ReviewThreadNotFound(asynxModels.ErrValidation)
	}
	if err != nil {
		return fmt.Errorf("reviewthread: delete thread: %w", err)
	}
	return nil
}

func (r *reviewThread) Resolve(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.ax.Send(ctx, commands.ResolveReviewThread{ID: id})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: resolve: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Reopen(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.ax.Send(ctx, commands.ReopenReviewThread{ID: id})
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
	if errors.Is(err, asynxModels.ErrNotFound) {
		return domain.ReviewThread{}, domain.ReviewThreadNotFound(err)
	}
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
