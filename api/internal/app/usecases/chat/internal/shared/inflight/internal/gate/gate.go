// Package gate serialises work on one key (a chat, a runner) and lets a Stop
// preempt whoever is parked holding it.
package gate

import (
	"context"
	"errors"
	"sync"
)

// ErrPreempted is the cause a holder's park context is cancelled with when a
// Preempt call wants the gate. A holder that sees it gives up without having
// changed anything.
var ErrPreempted = errors.New("gate: preempted by a stop")

// Gate is a per-key mutex whose waits honour a context, and whose holders can
// be asked to let go.
//
// The chat usecase builds three: the per-chat SPAWN gate (every user-initiated
// path that starts, replaces, attaches, prompts or purges a chat's CLI), the
// per-chat TURN-START gate (a hook's durable turn start versus a switch's final
// idle check) and the per-runner HOOK gate (one runner's hook ingestion).
//
// Starting a CLI is not a database write, so nothing in the persistence layer
// can order two of them: two concurrent switches on one chat would both read
// the same live runner, both kill it and both spawn. The gate is what stops
// that. It rejects nothing and reads no aggregate state.
//
// A holder may PARK while holding it — a provider switch waits, bounded, for
// the outgoing CLI to finish its turn. Acquire hands such a holder a park
// context, and Preempt cancels it: Stop must never queue behind a switch
// waiting on the very turn Stop is there to end (invariant A2).
type Gate struct {
	mu    sync.Mutex
	gates map[string]*entry
}

// entry is one key's lock plus a reference count, so the map does not grow
// without bound across the life of a daemon.
type entry struct {
	sem  chan struct{}
	refs int
	// park cancels the current holder's park context; nil while the gate is
	// free or held by a caller that cannot be preempted.
	park context.CancelCauseFunc
	// preempting counts Preempt calls not yet holding the gate. A holder that
	// enters while one is pending gets an already-cancelled park context, so a
	// Stop cannot be starved by a queue of switches.
	preempting int
}

// New returns an empty gate.
func New() *Gate {
	return &Gate{gates: map[string]*entry{}}
}

func (g *Gate) ref(key string, preempt bool) *entry {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.gates[key]
	if !ok {
		e = &entry{sem: make(chan struct{}, 1)}
		g.gates[key] = e
	}
	e.refs++
	if preempt {
		e.preempting++
		if e.park != nil {
			e.park(ErrPreempted)
		}
	}
	return e
}

func (g *Gate) unref(key string, e *entry, preempt bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if preempt {
		e.preempting--
	}
	e.refs--
	if e.refs == 0 {
		delete(g.gates, key)
	}
}

func (g *Gate) acquire(
	ctx context.Context,
	key string,
	preempt bool,
) (context.Context, func(), error) {
	e := g.ref(key, preempt)
	select {
	case e.sem <- struct{}{}:
	case <-ctx.Done():
		g.unref(key, e, preempt)
		return nil, nil, ctx.Err()
	}

	park, cancel := context.WithCancelCause(ctx)
	g.mu.Lock()
	if preempt {
		e.preempting--
	} else {
		e.park = cancel
		if e.preempting > 0 {
			cancel(ErrPreempted)
		}
	}
	g.mu.Unlock()

	return park, func() {
		g.mu.Lock()
		if !preempt {
			e.park = nil
		}
		g.mu.Unlock()
		cancel(nil)
		<-e.sem
		// preempt was already counted down on entry.
		g.unref(key, e, false)
	}, nil
}

// Lock blocks until key's gate is free and returns the release func. It
// cannot be cancelled and its holder cannot be preempted, so it is only for
// short critical sections that never park.
func (g *Gate) Lock(key string) func() {
	_, release, _ := g.acquire(context.Background(), key, false)
	return release
}

// Acquire waits for key's gate or for ctx. It returns the holder's park
// context — ctx, additionally cancelled with ErrPreempted when a Preempt call
// wants the gate — and the release func. A holder passes park to every wait
// it may park on and ctx to everything else, so a preemption abandons a wait
// and never a half-done write.
func (g *Gate) Acquire(ctx context.Context, key string) (park context.Context, release func(), err error) {
	return g.acquire(ctx, key, false)
}

// Preempt cancels the current holder's park context, and that of anybody who
// enters before this caller does, then waits for the gate or ctx.
func (g *Gate) Preempt(ctx context.Context, key string) (release func(), err error) {
	_, release, err = g.acquire(ctx, key, true)
	return release, err
}

// Preempted reports whether park was cancelled by a Preempt call.
func Preempted(park context.Context) bool {
	return errors.Is(context.Cause(park), ErrPreempted)
}
