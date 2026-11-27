package terminal

import "sync"

type reapTracker struct {
	births   sync.RWMutex
	mu       sync.Mutex
	n        int
	draining bool
	idle     chan struct{}
}

func (t *reapTracker) admit() bool {
	t.births.RLock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.draining {
		t.births.RUnlock()
		return false
	}
	t.n++
	return true
}

func (t *reapTracker) settle(spawned bool) {
	if !spawned {
		t.done()
	}
	t.births.RUnlock()
}

func (t *reapTracker) done() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.n--
	if t.n == 0 && t.idle != nil {
		close(t.idle)
		t.idle = nil
	}
}

func (t *reapTracker) drain() <-chan struct{} {
	t.births.Lock()
	defer t.births.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.draining = true
	if t.n == 0 {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	if t.idle == nil {
		t.idle = make(chan struct{})
	}
	return t.idle
}
