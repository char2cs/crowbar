// Package drain provides the shutdown gate every post-commit reactor enters before it
// spawns work, and that the app layer closes and waits out on the way down.
package drain

import (
	"context"
	"sync"
)

// Gate admits reactor goroutines until a drain begins, then refuses them and waits for
// the ones already inside to come home.
//
// It is a counter under a mutex, NOT a sync.WaitGroup, and that is the whole point. A
// reactor calls Add on asynx's bus goroutine the moment an event lands; shutdown calls
// Wait on its own. "Add called concurrently with Wait" is documented WaitGroup misuse —
// the race detector flags it, and the WaitGroup is entitled to PANIC on it outright.
//
// That is not hypothetical. `-race` caught it on the workspace delete reactor as soon as
// graceful shutdown began killing PTYs as its FIRST step: those deaths COMMIT EVENTS, and
// events wake reactors, so the daemon is at its most eventful in the very instant it is
// trying to go quiet.
//
// Cancelling a context does not fix it. A cancelled ctx tells a reactor to stop starting
// work, but the reactor still has to LOOK — and the whole race lives between its look and
// its Add. So admitting work and beginning to drain are made the SAME critical section:
// once draining, Enter refuses, the outstanding set can only shrink, and the drain always
// converges.
type Gate struct {
	mu       sync.Mutex
	n        int
	draining bool

	// idle is non-nil only while somebody is waiting for n to reach 0, and is CLOSED
	// (not sent on) so any number of waiters wake and a late waiter never blocks.
	idle chan struct{}
}

// New returns an open gate.
func New() *Gate {
	return &Gate{}
}

// Enter admits one goroutine, reporting false once a drain has begun — in which case the
// caller must NOT spawn the work. Being refused is not a dropped event: the daemon is
// going down, the databases the work would write to are about to close, and the boot sweep
// is what picks the job back up.
func (g *Gate) Enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.draining {
		return false
	}
	g.n++
	return true
}

// Leave retires a goroutine admitted by Enter, releasing any waiter once the last one is
// home. Pair it with `defer` at the top of the spawned goroutine, never at the call site:
// the point is to count the goroutine's LIFETIME, not the dispatch that created it.
func (g *Gate) Leave() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.n--
	if g.n == 0 && g.idle != nil {
		close(g.idle)
		g.idle = nil
	}
}

// WaitIdle blocks until no admitted goroutine remains, WITHOUT closing the gate — the gate
// keeps admitting, so this is a "has the work landed yet" barrier rather than a shutdown.
// It is what a test blocks on instead of guessing at a sleep.
//
// ctx is an escape hatch for a caller that has its own deadline, not a synchronisation
// device: pass context.Background() and let `go test -timeout` be the backstop.
func (g *Gate) WaitIdle(
	ctx context.Context,
) {
	g.mu.Lock()
	if g.n == 0 {
		g.mu.Unlock()
		return
	}
	if g.idle == nil {
		g.idle = make(chan struct{})
	}
	idle := g.idle
	g.mu.Unlock()

	select {
	case <-idle:
	case <-ctx.Done():
	}
}

// Wait closes the gate and blocks until every admitted goroutine has left, or ctx is done
// — a stuck reactor delays shutdown by the caller's deadline and no longer. Closing is
// idempotent, so a second Wait simply waits on the same, already-shrinking set.
func (g *Gate) Wait(
	ctx context.Context,
) {
	g.mu.Lock()
	g.draining = true
	g.mu.Unlock()

	g.WaitIdle(ctx)
}
