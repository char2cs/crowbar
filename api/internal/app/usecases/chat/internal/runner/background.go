package runner

import (
	"context"
	"sync"
)

// backgroundWork is the supervisor's own follow-up work (a native view's
// exit handling): tracked, so Shutdown cancels it and waits, and none outlives
// the daemon. A nil one (a bare Runners built by a test) runs untracked.
type backgroundWork struct {
	mu      sync.Mutex
	stopped bool
	cancels map[*context.CancelFunc]struct{}
	wg      sync.WaitGroup
}

func newBackgroundWork() *backgroundWork {
	return &backgroundWork{cancels: map[*context.CancelFunc]struct{}{}}
}

// run starts fn with parent's values but the supervisor's lifetime: it
// outlives the request that asked for it and ends with Shutdown.
func (b *backgroundWork) run(parent context.Context, fn func(context.Context)) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	if b == nil {
		go func() {
			defer cancel()
			fn(ctx)
		}()
		return
	}
	b.mu.Lock()
	if b.stopped {
		cancel() // after Shutdown it runs already cancelled, and is still waited for
	}
	b.cancels[&cancel] = struct{}{}
	b.wg.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wg.Done()
		defer b.forget(&cancel)
		fn(ctx)
	}()
}

func (b *backgroundWork) forget(cancel *context.CancelFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.cancels, cancel)
	(*cancel)()
}

// stop cancels every piece of background work and waits for it to return.
func (b *backgroundWork) stop() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stopped = true
	for cancel := range b.cancels {
		(*cancel)()
	}
	b.mu.Unlock()
	b.wg.Wait()
}
