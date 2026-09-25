// Package detached owns the work a handler hands off after answering 202: the
// one place that work is started, and the one place a daemon shutdown waits for
// it, so no such op — nor the git process it runs — outlives the daemon.
package detached

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// cancelGrace bounds how long Shutdown waits for cancelled ops to return. A git
// subprocess is killed on cancel, so they return well within it.
const cancelGrace = time.Second

// Ops tracks detached ops. The zero value is ready to use.
type Ops struct {
	wg sync.WaitGroup

	mu        sync.Mutex
	cancels   map[*context.CancelFunc]struct{}
	cancelled bool
}

// Go runs fn in a goroutine on a ctx that outlives the request (parent is
// cancelled once the 202 is flushed) and ends only when Shutdown gives up
// waiting. A panic is contained and logged under name.
func (o *Ops) Go(parent context.Context, name string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	o.track(&cancel)
	// Add runs on the caller's goroutine, before the spawn, so a Wait that
	// happens-after the handler returned can never miss this op.
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		defer o.untrack(&cancel)
		defer safego.Recover(name)
		fn(ctx)
	}()
}

func (o *Ops) track(cancel *context.CancelFunc) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cancelled {
		(*cancel)()
		return
	}
	if o.cancels == nil {
		o.cancels = map[*context.CancelFunc]struct{}{}
	}
	o.cancels[cancel] = struct{}{}
}

func (o *Ops) untrack(cancel *context.CancelFunc) {
	(*cancel)()
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.cancels, cancel)
}

func (o *Ops) cancelAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cancelled = true
	for cancel := range o.cancels {
		(*cancel)()
	}
}

// Wait blocks until every op started so far has returned.
func (o *Ops) Wait() { o.wg.Wait() }

// Shutdown waits for every op to return. If ctx ends first it cancels them —
// killing any subprocess they run — and waits up to cancelGrace for them. Call
// it once no new op can start (the HTTP server has stopped).
func (o *Ops) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		o.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
	}
	o.cancelAll()
	timer := time.NewTimer(cancelGrace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	return fmt.Errorf("detached: shutdown: cancelled what was still running: %w", ctx.Err())
}
