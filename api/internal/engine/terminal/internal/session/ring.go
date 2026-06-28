// Package session provides PTY session management with ring-buffer output history.
package session

import (
	"io"
	"sync"
)

// defaultRingSize is the per-session scrollback budget.
// 100-session ceiling × 256 KB = 25.6 MB total peak memory.
const defaultRingSize = 256 * 1024 // 256 KB

// DefaultRingSize is the per-session scrollback ring capacity, exported for
// the engine to compute total ring-memory ceilings.
const DefaultRingSize = defaultRingSize

// RingBuffer is a circular byte buffer that retains the most recent N bytes.
// All methods are safe for concurrent use.
type RingBuffer struct {
	mu   sync.Mutex
	buf  []byte
	head int
	size int
}

// newRingBuffer allocates a RingBuffer with the given capacity.
func newRingBuffer(
	capacity int,
) *RingBuffer {
	return &RingBuffer{buf: make([]byte, capacity)}
}

// Write appends p into the ring, overwriting the oldest bytes when full.
func (r *RingBuffer) Write(
	p []byte,
) {
	if len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cap := len(r.buf)

	// If p is larger than the buffer, only keep the last cap bytes.
	if len(p) >= cap {
		p = p[len(p)-cap:]
		copy(r.buf, p)
		r.head = 0
		r.size = cap
		return
	}

	// How many bytes fit before we wrap around?
	tail := cap - r.head
	if len(p) <= tail {
		n := copy(r.buf[r.head:], p)
		r.head = (r.head + n) % cap
	} else {
		copy(r.buf[r.head:], p[:tail])
		rest := p[tail:]
		copy(r.buf, rest)
		r.head = len(rest)
	}
	r.size = min(r.size+len(p), cap)
}

// Snapshot returns a copy of all buffered bytes in chronological order.
func (r *RingBuffer) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

// snapshotLocked returns the chronological snapshot without acquiring mu.
// Callers must hold r.mu.
func (r *RingBuffer) snapshotLocked() []byte {
	if r.size == 0 {
		return nil
	}

	out := make([]byte, r.size)
	cap := len(r.buf)
	start := (r.head - r.size + cap) % cap

	if start+r.size <= cap {
		copy(out, r.buf[start:start+r.size])
		return out
	}

	firstChunk := cap - start
	copy(out, r.buf[start:])
	copy(out[firstChunk:], r.buf[:r.size-firstChunk])
	return out
}

// Flush writes all buffered bytes in chronological order to w.
// It is safe for concurrent use.
func (r *RingBuffer) Flush(w io.Writer) error {
	r.mu.Lock()
	data := r.snapshotLocked()
	r.mu.Unlock()

	if len(data) == 0 {
		return nil
	}
	_, err := w.Write(data)
	return err
}

// Load reads all bytes from rd and writes them into the ring, respecting
// capacity (only the last capacity bytes are retained, same as Write).
// It is safe for concurrent use; there is no double-lock because the mutex
// is never held while calling r.Write.
func (r *RingBuffer) Load(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	r.Write(data)
	return nil
}
