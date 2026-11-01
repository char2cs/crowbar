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

// BroadcastFunc is an alias for the store-layer broadcast type, exposed so the
// repositories container can wire it without importing the internal store package.
type BroadcastFunc = store.BroadcastFunc

// OpenInput carries the anchor + first message for a new thread (09 §3).
//
// ProviderID and ChatID attribute the first message to the agent that wrote it
// and to the chat it came out of. The human/UI path leaves both empty: a person
// has neither a provider nor an originating conversation.
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

// ReplyInput carries the message appended to an existing thread (09 §3).
//
// It is a struct rather than the positional argument list it replaces because
// agent attribution took the parameter count past what a call site can be read
// at: seven anonymous strings and bools in a row, two of them newly added and
// both optional, is a signature where a transposed pair compiles and silently
// files a finding under the wrong chat.
//
// ProviderID and ChatID carry the same meaning they do on OpenInput, and are
// empty for the same human/UI path.
type ReplyInput struct {
	ID         string
	MessageID  string
	Author     string
	IsAgent    bool
	ProviderID string
	ChatID     string
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

// reviewThread is the singleton-backed review-thread aggregate repository. One
// axReviewThread routes every thread id to a shard by hash; there is no writeMu
// (asynx serializes each aggregate's commands).
type reviewThread struct {
	ax    asynx.Asynx[domain.ReviewThread]
	store store.Store
}

// ReadModel is the read model's write side (state/store/review_thread.db). It is opened before the
// asynx instance so it can be that instance's durable subscriber.
type ReadModel = store.Projection

// OpenReadModel opens the read model over storeDB.
func OpenReadModel(
	storeDB *gormdb.DB,
) (*ReadModel, error) {
	rm, err := store.Open(storeDB)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: read model: %w", err)
	}
	return rm, nil
}

// Subscriber is the builder option that makes rm the instance's durable subscriber.
func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.ReviewThread]) {
	return store.Subscriber(rm)
}

// New builds the repository over ax, which must have been built with
// Subscriber(rm), and the hub broadcast fan-out. es is the log ax wraps,
// kept so the read model can heal itself by lazy Replay (§3.7).
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

// DeleteThread purges the thread aggregate via Forget: its synchronous OnForget
// deletes the read-model row (a cheap, bounded row-delete — spec §3.6). A review
// thread has no worktree/cascade, so unlike Workspace it needs no tombstone
// command or async delete reactor.
func (r *reviewThread) DeleteThread(
	ctx context.Context,
	id string,
) error {
	err := r.ax.Forget(ctx, id)
	// Forget's one refusal is an aggregate it does not hold.
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
	// Per-id reads fold the aggregate directly from the event log (§3.7), so Get is
	// always current and needs no read-model rebuild.
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
