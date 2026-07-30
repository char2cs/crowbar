package agenttools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
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
type Idempotency struct {
	mu     sync.Mutex
	opened map[idempotencyRef]string
}

type idempotencyRef struct {
	wsID string
	key  string
}

// NewIdempotency builds the per-daemon dedup map. The container calls it once.
func NewIdempotency() *Idempotency {
	return &Idempotency{opened: map[idempotencyRef]string{}}
}

// OpenOnce opens in through writer and returns the new thread's id, unless a
// previous call carrying the same key for the same workspace already opened one —
// in which case it returns THAT thread's id and writes nothing.
//
// An empty key means the caller asked for no deduplication, so the write happens
// unguarded.
//
// The lock is held across the write rather than only around the map lookups:
// two concurrent retries of one finding would otherwise both miss the map and
// both write, which is the exact duplicate the key exists to prevent. That
// serializes keyed posts against each other, which is irrelevant at the rate an
// agent posts findings and is worth the guarantee.
func (i *Idempotency) OpenOnce(
	ctx context.Context,
	writer ThreadWriter,
	key string,
	in reviewthread.OpenInput,
	now time.Time,
) (string, error) {
	if key == "" {
		return openThread(ctx, writer, in, now)
	}
	ref := idempotencyRef{wsID: in.WsID, key: key}

	i.mu.Lock()
	defer i.mu.Unlock()

	if id, ok := i.opened[ref]; ok {
		return id, nil
	}
	id, err := openThread(ctx, writer, in, now)
	if err != nil {
		return "", err
	}
	i.opened[ref] = id
	return id, nil
}

func openThread(
	ctx context.Context,
	writer ThreadWriter,
	in reviewthread.OpenInput,
	now time.Time,
) (string, error) {
	thread, err := writer.Open(ctx, in, now)
	if err != nil {
		return "", fmt.Errorf("agenttools: open review thread: %w", err)
	}
	return thread.ID, nil
}
