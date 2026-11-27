package tools

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Idempotency struct {
	mu      sync.Mutex
	opened  map[idempotencyRef]openedThread
	replied map[idempotencyRef]string
}

type idempotencyRef struct {
	wsID string
	key  string
}

type openedThread struct {
	ID        string
	FilePath  string
	StartLine int
	EndLine   int
	Side      domain.ReviewSide
}

type openOutcome struct {
	stored  openedThread
	fresh   domain.ReviewThread
	created bool
}

type replyOutcome struct {
	threadID string
	fresh    domain.ReviewThread
	created  bool
}

var errNoDedupMap = errors.New("idempotencyKey cannot be honoured: this daemon has no dedup map")

func NewIdempotency() *Idempotency {
	return &Idempotency{
		opened:  map[idempotencyRef]openedThread{},
		replied: map[idempotencyRef]string{},
	}
}

func (i *Idempotency) openOnce(
	ctx context.Context,
	writer ThreadWriter,
	key string,
	in reviewthread.OpenInput,
	now time.Time,
) (openOutcome, error) {
	if key == "" {
		return open(ctx, writer, in, now)
	}
	if i == nil {
		return openOutcome{}, errNoDedupMap
	}
	ref := idempotencyRef{wsID: in.WsID, key: key}

	i.mu.Lock()
	defer i.mu.Unlock()

	if stored, ok := i.opened[ref]; ok {
		return openOutcome{stored: stored}, nil
	}
	out, err := open(ctx, writer, in, now)
	if err != nil {
		return openOutcome{}, err
	}
	i.opened[ref] = out.stored
	return out, nil
}

func (i *Idempotency) replyOnce(
	ctx context.Context,
	writer ThreadWriter,
	wsID string,
	key string,
	in reviewthread.ReplyInput,
	now time.Time,
) (replyOutcome, error) {
	if key == "" {
		return reply(ctx, writer, in, now)
	}
	if i == nil {
		return replyOutcome{}, errNoDedupMap
	}
	ref := idempotencyRef{wsID: wsID, key: key}

	i.mu.Lock()
	defer i.mu.Unlock()

	if threadID, ok := i.replied[ref]; ok {
		return replyOutcome{threadID: threadID}, nil
	}
	out, err := reply(ctx, writer, in, now)
	if err != nil {
		return replyOutcome{}, err
	}
	i.replied[ref] = out.threadID
	return out, nil
}

func reply(
	ctx context.Context,
	writer ThreadWriter,
	in reviewthread.ReplyInput,
	now time.Time,
) (replyOutcome, error) {
	thread, err := writer.Reply(ctx, in, now)
	if err != nil {
		return replyOutcome{}, err
	}
	return replyOutcome{threadID: in.ID, fresh: thread, created: true}, nil
}

func open(
	ctx context.Context,
	writer ThreadWriter,
	in reviewthread.OpenInput,
	now time.Time,
) (openOutcome, error) {
	thread, err := writer.Open(ctx, in, now)
	if err != nil {
		return openOutcome{}, err
	}
	return openOutcome{
		stored: openedThread{
			ID:        thread.ID,
			FilePath:  thread.FilePath,
			StartLine: thread.StartLine,
			EndLine:   thread.EndLine,
			Side:      thread.Side,
		},
		fresh:   thread,
		created: true,
	}, nil
}
