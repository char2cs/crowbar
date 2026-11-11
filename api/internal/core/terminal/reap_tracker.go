package terminal

import "sync"

// reapTracker makes reaps (which write to databases shutdown will close) a step of shutdown. Not a
// WaitGroup, which panics on Add concurrent with Wait; once draining, admit refuses. births is held
// from admit to settle so drain cannot close while a child is spawned but not yet registered.
type reapTracker struct {
	births   sync.RWMutex
	mu       sync.Mutex
	n        int
	draining bool
	idle     chan struct{} // non-nil only while a drain is waiting on n > 0
}

// admit opens a birth and claims its reap slot, reporting false once a drain has begun.
// A true result must be followed by exactly one settle.
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

// settle closes a birth opened by admit. A birth whose child never started (spawned ==
// false) returns its reap slot, since no reaper will ever retire it.
func (t *reapTracker) settle(spawned bool) {
	if !spawned {
		t.done()
	}
	t.births.RUnlock()
}

// done retires a reap goroutine, releasing a waiting drain once the last one is home.
func (t *reapTracker) done() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.n--
	if t.n == 0 && t.idle != nil {
		close(t.idle)
		t.idle = nil
	}
}

// drain closes the door on new reaps and returns a channel closed once every outstanding
// one has returned. Idempotent.
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
