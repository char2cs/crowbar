package agenttools

import (
	"context"
	"errors"
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
//
// Opening and replying keep SEPARATE namespaces. A model naming a finding
// "nil-deref-in-auth" when it posts it and again when it replies about it is
// naming the same finding, not the same write, and one map would let the reply be
// swallowed as a retry of the post — or, worse, answered with the post's cached
// anchor. Two maps under one mutex means a key means "this post" or "this reply"
// and never both.
type Idempotency struct {
	mu      sync.Mutex
	opened  map[idempotencyRef]openedThread
	replied map[idempotencyRef]string
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

// replyOutcome is what one replyOnce attempt produced, shaped like openOutcome and
// for the same reason: fresh is set only when created is true, so "there is an
// aggregate to announce only when THIS call wrote one" is a fact about the type
// rather than a comment a caller has to remember.
//
// threadID is the thread the reply LANDED on, cached so a retry is answered with
// it rather than with its own argument — the reply counterpart of openOutcome's
// stored anchor. A retry that reuses a key against a different thread wrote
// nothing, and naming the thread it asked for would report a reply nobody can find.
type replyOutcome struct {
	threadID string
	fresh    domain.ReviewThread
	created  bool
}

// errNoDedupMap is what a KEYED write gets when the daemon was wired without an
// Idempotency map at all.
//
// It fails rather than writing unguarded because the key is a promise: a caller
// that supplied one is retrying, and honouring the write while ignoring the key is
// exactly the duplicate the key was passed to prevent. An UNKEYED write is
// unaffected — it never asked for dedup — which is why reply_to_review_thread and
// resolve_review_thread stay registered without a map. Gating their registration
// on it instead would also suppress resolve_review_thread, which is idempotent by
// nature and has no use for a dedup map.
var errNoDedupMap = errors.New("idempotencyKey cannot be honoured: this daemon has no dedup map")

// NewIdempotency builds the per-daemon dedup maps. The container calls it once.
func NewIdempotency() *Idempotency {
	return &Idempotency{
		opened:  map[idempotencyRef]openedThread{},
		replied: map[idempotencyRef]string{},
	}
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
	// Symmetric with replyOnce, and for the same reason: a KEYED write on a daemon
	// wired without a dedup map is refused, never written unguarded. Unreachable
	// today — post_review_comment is not even registered without one (see
	// canPostReviewComment) — but the alternative to the guard is a nil-deref on
	// the map below, and the two halves of one mechanism disagreeing about a case
	// is how the reachable version of that gets written later.
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
	// A FAILED write is deliberately not recorded: the retry this key exists for
	// must still reach the store, rather than be answered with a thread that was
	// never opened.
	if err != nil {
		return openOutcome{}, err
	}
	i.opened[ref] = out.stored
	return out, nil
}

// replyOnce appends in through writer, unless a previous call carrying the same key
// from the same workspace already replied — in which case it returns THAT reply's
// thread and writes nothing.
//
// The trigger is concrete, not hypothetical: the MCP relay's writeLine can fail on
// a broken pipe AFTER the daemon has committed the reply. The relay keeps serving,
// the client never saw a result, and it retries — into a second copy of the same
// reply sitting under the user's finding.
//
// wsID is passed rather than read off in because ReplyInput carries no workspace:
// a reply names a thread, and that thread can live on a DESCENDANT of the caller's
// workspace. It is deliberately the CALLER's workspace, exactly as the open path
// keys on the workspace of the agent doing the writing. Keying on the thread's
// workspace instead would let two agents on two branches, both replying to one
// shared descendant thread with the same obvious key ("acknowledged"), collapse
// into a single reply — silently discarding the second agent's answer, which is a
// worse failure than the duplicate this whole mechanism exists to prevent.
//
// An empty key means the caller asked for no deduplication, so the write happens
// unguarded — and that path touches no map, which is what keeps an unkeyed reply
// working on a daemon wired without one.
//
// The lock is held across the write for the same reason openOnce holds it: two
// concurrent retries of one reply would otherwise both miss the map and both write.
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
	// A FAILED write is deliberately not recorded, exactly as in openOnce: the retry
	// this key exists for must still reach the store.
	if err != nil {
		return replyOutcome{}, err
	}
	i.replied[ref] = out.threadID
	return out, nil
}

// reply performs the unguarded write.
//
// The thread id recorded is the one the caller NAMED — the id its authorization
// was checked against — rather than the id on the returned aggregate: the two are
// the same thread, and the named one is the only one guaranteed to be present
// whatever a store returns.
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
