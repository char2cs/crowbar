// Package session provides PTY session management with ring-buffer output history.
package session

import (
	"io"
	"sync"
)

// defaultRingSize is the per-session scrollback budget (the ceiling a ring
// grows toward, not an eager allocation — see newRingBuffer/growLocked). A
// busy TUI like Claude Code emits scrollback faster than the old 256 KB held,
// so a re-attach after a workspace switch dropped the top of the session. 1 MiB
// retains ~4× more while the lazy backing array keeps idle/short sessions tiny.
// Worst case is bounded: 100-session ceiling × 1 MiB = 100 MB peak, reached only
// if every session actually fills its budget.
const defaultRingSize = 1024 * 1024 // 1 MiB

// DefaultRingSize is the per-session scrollback ring capacity, exported for
// the engine to compute total ring-memory ceilings.
const DefaultRingSize = defaultRingSize

// initialRingAlloc is the starting backing-array size for a ring whose ceiling
// exceeds it. The ring grows from here toward maxCap as output accumulates, so a
// session that prints little never pays for the full budget.
const initialRingAlloc = 4096

// RingBuffer is a circular byte buffer that retains the most recent maxCap
// bytes. The backing array grows lazily from initialRingAlloc toward maxCap, so
// memory tracks actual output. All methods are safe for concurrent use.
type RingBuffer struct {
	mu     sync.Mutex
	buf    []byte
	head   int
	size   int
	maxCap int // logical capacity ceiling; len(buf) grows toward it
}

// newRingBuffer allocates a RingBuffer whose retained-byte ceiling is maxCap.
// The backing array starts small (initialRingAlloc) and grows lazily, except
// when maxCap is already smaller — then it is allocated eagerly at maxCap.
func newRingBuffer(
	maxCap int,
) *RingBuffer {
	if maxCap < 1 {
		maxCap = 1
	}
	init := maxCap
	if init > initialRingAlloc {
		init = initialRingAlloc
	}
	return &RingBuffer{buf: make([]byte, init), maxCap: maxCap}
}

// growLocked enlarges the backing array toward maxCap when `needed` bytes would
// not fit, re-linearizing existing data to start at index 0. It is a no-op once
// len(buf) has reached maxCap — from then on the ring wraps like a fixed buffer.
// Caller must hold r.mu.
func (r *RingBuffer) growLocked(
	needed int,
) {
	if len(r.buf) >= r.maxCap || needed <= len(r.buf) {
		return
	}
	newCap := needed
	if newCap > r.maxCap {
		newCap = r.maxCap
	}
	nb := make([]byte, newCap)
	n := copy(nb, r.snapshotLocked()) // chronological copy; n == r.size
	r.buf = nb
	r.size = n
	r.head = n % newCap
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

	// Grow toward maxCap before writing so a chunk that would overflow the
	// current (sub-ceiling) allocation does not evict still-fitting history.
	r.growLocked(r.size + len(p))

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

// Cap returns the logical byte capacity (ceiling) of the ring. The backing
// array may currently be smaller — it grows lazily — but Cap reports the maximum
// scrollback the ring will retain. maxCap is fixed at construction, so this is
// safe to read without holding r.mu.
func (r *RingBuffer) Cap() int {
	return r.maxCap
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
