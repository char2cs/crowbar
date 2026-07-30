package agenttools

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Idempotency remembers which review thread each (workspace, key) pair already
// opened, so a write tool a model retries — a dropped MCP response, a re-run of
// the same review pass — leaves ONE thread rather than a duplicate finding.
//
// It is owned by Deps and constructed exactly once per daemon, NOT held on a
// ToolSet: a ToolSet is built per MCP request, so a map living there would be
// thrown away between the original call and its retry and the key would be
// inert. Because Deps is copied by value in places (the agent usecase fills its
// own Chats port in on a copy), this must be reached through a pointer for every
// copy to share one map.
//
// The keys are scoped by workspace id so two agents reviewing two branches
// cannot collide on the same obvious key ("nil-deref-in-auth") and have the
// second finding silently swallowed as a retry of the first.
//
// It caches each thread's ANCHOR, not the thread, so a retry can be answered with
// the location that was actually STORED: a retry that reuses a key but changes the
// lines wrote nothing, and answering it from its own arguments would report a post
// at a location no thread is anchored to.
type Idempotency struct {
	mu     sync.Mutex
	opened map[idempotencyRef]openedThread
}

type idempotencyRef struct {
	wsID string
	key  string
}

// openedThread is the cached value: only the fields a retry's reply consumes.
//
// The thread's MESSAGES are deliberately not retained. Nothing reads them — the
// broadcast fires only for a genuine write and carries the fresh aggregate from the
// store, never a cache entry — and this map is unbounded and lives as long as the
// process, so holding user-authored review bodies in it would be retention with no
// consumer.
type openedThread struct {
	ID        string
	FilePath  string
	StartLine int
	EndLine   int
	Side      domain.ReviewSide
}

// openOutcome is what one OpenOnce attempt produced.
//
// fresh is separate from stored, and set only when created is true, so "the full
// aggregate exists only when THIS call wrote it" is a fact about the type rather
// than a comment: a caller cannot reach for a whole thread on a dedup hit, because
// on a dedup hit there is not one to reach for.
type openOutcome struct {
	stored  openedThread
	fresh   domain.ReviewThread
	created bool
}

// NewIdempotency builds the per-daemon dedup map. The container calls it once.
func NewIdempotency() *Idempotency {
	return &Idempotency{opened: map[idempotencyRef]openedThread{}}
}

// openOnce opens in through writer, unless a previous call carrying the same key for
// the same workspace already opened a thread — in which case it returns THAT
// thread's anchor and writes nothing.
//
// An empty key means the caller asked for no deduplication, so the write happens
// unguarded.
//
// The lock is held across the write rather than only around the map lookups:
// two concurrent retries of one finding would otherwise both miss the map and
// both write, which is the exact duplicate the key exists to prevent. That
// serializes keyed posts against each other, which is irrelevant at the rate an
// agent posts findings and is worth the guarantee.
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
	ref := idempotencyRef{wsID: in.WsID, key: key}

	i.mu.Lock()
	defer i.mu.Unlock()

	if stored, ok := i.opened[ref]; ok {
		return openOutcome{stored: stored}, nil
	}
	out, err := open(ctx, writer, in, now)
	// A FAILED write is deliberately not recorded: the retry this key exists for
	// must still reach the store, rather than be answered with a thread that was
	// never opened.
	if err != nil {
		return openOutcome{}, err
	}
	i.opened[ref] = out.stored
	return out, nil
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
