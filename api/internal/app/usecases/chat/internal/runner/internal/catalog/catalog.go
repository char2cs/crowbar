// Package catalog bounds the deterministic slash-command probes Crowbar runs
// against a vendor CLI.
//
// A probe forks the provider's binary to ask what commands it offers. Two things
// therefore need bounding: the same chat asking twice (the older probe is
// pointless the moment a newer one starts) and the daemon as a whole (a panel of
// eight chats must not fork eight CLIs at once).
//
// Results are deliberately never cached. A catalog changes when the user edits
// their own command files, and a stale list is worse than a slow one.
package catalog

import (
	"context"
	"sync"
)

// maxProcesses is the daemon-wide budget of concurrently forked probes.
const maxProcesses = 4

type run struct {
	id     uint64
	cancel context.CancelFunc
}

// Runs owns cancellation for in-flight probes: one current probe per chat, and a
// shared process budget across every chat.
type Runs struct {
	mu           sync.Mutex
	nextID       uint64
	runs         map[string]run
	processSlots chan struct{}
}

// New returns an empty run registry with a full process budget.
func New() *Runs {
	return &Runs{
		runs:         map[string]run{},
		processSlots: make(chan struct{}, maxProcesses),
	}
}

// Start opens a probe for chatID, cancelling whichever probe that chat already
// had. The returned finish func cancels this probe and retires it — but only if
// it is still the current one, so a probe that has already been superseded does
// not evict its own replacement.
func (r *Runs) Start(parent context.Context, chatID string) (context.Context, func()) {
	r.mu.Lock()
	if old, ok := r.runs[chatID]; ok {
		old.cancel()
	}
	r.nextID++
	id := r.nextID
	ctx, cancel := context.WithCancel(parent)
	r.runs[chatID] = run{id: id, cancel: cancel}
	r.mu.Unlock()
	return ctx, func() {
		cancel()
		r.mu.Lock()
		if current, ok := r.runs[chatID]; ok && current.id == id {
			delete(r.runs, chatID)
		}
		r.mu.Unlock()
	}
}

// AcquireProcess takes one slot of the daemon-wide fork budget, blocking until
// one is free or ctx is done. The returned release is idempotent, so a caller may
// defer it and still release early.
func (r *Runs) AcquireProcess(ctx context.Context) (func(), error) {
	select {
	case r.processSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-r.processSlots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
