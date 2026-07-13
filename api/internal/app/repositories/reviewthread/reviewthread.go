package reviewthread

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed
// (decision 10): with writeMu deleted, concurrent Sends to one thread aggregate
// version-collide, so the losers retry — Send re-reads the current version each
// attempt, so a retry converges. ErrValidation is NEVER retried; ErrQueueFull is
// surfaced as apperr.ErrUnavailable.
//
// The bound must exceed the worst-case number of concurrent writers to a single
// aggregate: asynx (v0.7.0+) enforces real OCC — each version slot is won by
// exactly one writer, so N contenders converge in at most N attempts (monotonic
// progress, not timing-dependent). Before v0.7.0 the collision never fired, so
// this was effectively untested; the ConcurrentReplies stress test (n=32) now
// exercises it for real. 64 covers any realistic same-thread reply burst with
// margin; production contention on one thread is a handful.
const maxOCCAttempts = 64

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

// reviewThread is the singleton-backed review-thread aggregate repository. One
// axReviewThread routes every thread id to a shard by hash; there is no writeMu
// (per-aggregate safety is shard routing + (id,version) uniqueness + OCC retry).
type reviewThread struct {
	ax    asynx.Asynx[domain.ReviewThread]
	store store.Store
}

// New builds the singleton-backed ReviewThread repository over axReviewThread, the
// reviewthread read-model DB (state/store/review_thread.db), and the hub broadcast
// fan-out. es is the per-type event log axReviewThread wraps
// (state/events/review_thread.db), retained so the read model can heal itself via
// whole-model lazy Replay on first access after a loss (§3.7). It registers the
// save-only store projection AND the hub projection on axReviewThread via
// store.New.
func New(
	ax asynx.Asynx[domain.ReviewThread],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	broadcast BroadcastFunc,
) (ReviewThread, error) {
	st, err := store.New(storeDB, es, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("reviewthread: store: %w", err)
	}
	return &reviewThread{ax: ax, store: st}, nil
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.ReviewThread],
) (asynxModels.Event[domain.ReviewThread], error)

// occSend runs send with OCC retry and the terminal error disposition contract
// (spec §3.5, decision 10):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing after
//     the retries is an unrecoverable optimistic-concurrency collision, surfaced
//     as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.ReviewThread],
) (asynxModels.Event[domain.ReviewThread], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.ReviewThread]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.ReviewThread]{}, fmt.Errorf("reviewthread: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[domain.ReviewThread]{}, err
		}
	}
	return asynxModels.Event[domain.ReviewThread]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axReviewThread with OCC retry.
func (r *reviewThread) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.ReviewThread],
) (asynxModels.Event[domain.ReviewThread], error) {
	return occSend(ctx, r.ax.Send, cmd)
}

func (r *reviewThread) Open(
	ctx context.Context,
	in OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.sendWithOCC(ctx, commands.OpenReviewThread{
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
	evt, err := r.sendWithOCC(ctx, commands.ReplyReviewThread{
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
	evt, err := r.sendWithOCC(ctx, commands.EditReviewMessage{
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
	evt, err := r.sendWithOCC(ctx, commands.DeleteReviewMessage{
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
	if err := r.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("reviewthread: delete thread: %w", err)
	}
	return nil
}

func (r *reviewThread) Resolve(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.sendWithOCC(ctx, commands.ResolveReviewThread{ID: id})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: resolve: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Reopen(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.sendWithOCC(ctx, commands.ReopenReviewThread{ID: id})
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
