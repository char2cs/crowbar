package tools

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Idempotency remembers which review write each (workspace, key) pair already made, so a retried tool
// call leaves one thread or reply, not a duplicate. It lives once per daemon behind a pointer on Deps,
// not on the per-request ToolSet, so a retry and its original share one map.
type Idempotency struct {
	mu sync.Mutex
	// opened caches the stored anchor, so a retry with changed lines reports where the thread really is.
	opened map[idempotencyRef]openedThread
	// replied is separate from opened: reusing a finding's key for a reply means the same finding,
	// not the same write.
	replied map[idempotencyRef]string
}

// idempotencyRef scopes keys by workspace, so two agents on two branches using the same obvious key
// cannot swallow each other's findings.
type idempotencyRef struct {
	wsID string
	key  string
}

// openedThread is the cached value: only the fields a retry's answer consumes. Messages are not kept:
// the map is unbounded and process-long, and nothing reads review bodies from it.
type openedThread struct {
	ID        string
	FilePath  string
	StartLine int
	EndLine   int
	Side      domain.ReviewSide
}

// openOutcome is what one openOnce attempt produced. fresh is set only when created is true: the full
// aggregate exists only when this call wrote it, never on a dedup hit.
type openOutcome struct {
	stored  openedThread
	fresh   domain.ReviewThread
	created bool
}

// replyOutcome is what one replyOnce attempt produced; fresh is set only when created is true. threadID
// is the thread the reply landed on, so a retry naming a different thread is answered truthfully.
type replyOutcome struct {
	threadID string
	fresh    domain.ReviewThread
	created  bool
}

// errNoDedupMap refuses a keyed write on a daemon wired without an Idempotency map: writing it unguarded
// would risk the very duplicate the key asks to prevent. Unkeyed writes are unaffected.
var errNoDedupMap = errors.New("idempotencyKey cannot be honoured: this daemon has no dedup map")

// NewIdempotency builds the per-daemon dedup maps. The container calls it once.
func NewIdempotency() *Idempotency {
	return &Idempotency{
		opened:  map[idempotencyRef]openedThread{},
		replied: map[idempotencyRef]string{},
	}
}

// openOnce opens in through writer unless the same workspace already opened a thread under key, in which
// case it returns that thread's anchor; an empty key skips dedup. The lock spans the write so two
// concurrent retries cannot both miss the map and both write.
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
	// post_review_comment is not registered without a map; the guard keeps this symmetric with
	// replyOnce rather than a nil-deref.
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
	// A failed write is not recorded, so its retry still reaches the store.
	if err != nil {
		return openOutcome{}, err
	}
	i.opened[ref] = out.stored
	return out, nil
}

// replyOnce appends in through writer unless the same workspace already replied under key, in which case
// it returns that reply's thread; an empty key skips dedup and the map. wsID is the caller's workspace,
// not the thread's, so two agents replying to one shared thread with the same key both land.
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
	// A failed write is not recorded, so its retry still reaches the store.
	if err != nil {
		return replyOutcome{}, err
	}
	i.replied[ref] = out.threadID
	return out, nil
}

// reply performs the unguarded write. It records the thread id the caller named (the one authorization
// checked), which is present whatever the store returns.
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
