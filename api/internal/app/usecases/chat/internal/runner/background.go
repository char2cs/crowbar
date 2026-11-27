package runner

import (
	"context"
	"sync"
)

type backgroundWork struct {
	mu      sync.Mutex
	stopped bool
	cancels map[*context.CancelFunc]struct{}
	wg      sync.WaitGroup
}

func newBackgroundWork() *backgroundWork {
	return &backgroundWork{cancels: map[*context.CancelFunc]struct{}{}}
}

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
		cancel()
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
