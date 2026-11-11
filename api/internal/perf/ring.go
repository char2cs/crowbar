// Package perf holds a bounded, race-free ring of timing samples from every layer, so latency can be
// attributed across boundaries on one clock. Off by default (one atomic load per Record) so the hot
// call sites can stay instrumented permanently without taxing what they measure.
package perf

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cap bounds the ring, sized for a whole scenario: a ring that wraps mid-run silently reports a
// truncated window as the whole story.
const Cap = 4096

// Sample is one timed operation: Name is the bucket, At its completion, so readers can nest
// subprocess samples under their request. Milliseconds, as JSON readers know no Go Duration.
type Sample struct {
	Name       string    `json:"name"`
	DurationMS float64   `json:"durationMs"`
	At         time.Time `json:"at"`
}

var (
	enabled atomic.Bool

	mu      sync.Mutex
	samples []Sample
)

// SetEnabled arms or disarms recording. Disabled is the default.
func SetEnabled(
	v bool,
) {
	enabled.Store(v)
}

// Enabled reports whether recording is armed. Call sites that must do setup work
// before they can time anything — capturing a start instant, wrapping a call —
// check this first so the disarmed path stays a single atomic load.
func Enabled() bool {
	return enabled.Load()
}

// Record appends one sample under name, evicting the oldest once the ring is
// full. It is a no-op — one atomic load, no lock taken — while disabled.
func Record(
	name string,
	d time.Duration,
) {
	if !enabled.Load() {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	samples = append(samples, Sample{
		Name:       name,
		DurationMS: float64(d.Nanoseconds()) / 1e6,
		At:         time.Now(),
	})
	if len(samples) > Cap {
		samples = samples[len(samples)-Cap:]
	}
}

// Snapshot returns a copy of the ring, oldest first. The copy is load-bearing:
// readers serialise and post-process the result outside the lock, and handing
// back the live slice would race every concurrent Record.
func Snapshot() []Sample {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Sample, len(samples))
	copy(out, samples)
	return out
}

// Reset clears the ring. A capture run resets as it arms, so the measurement
// window starts empty and no sample left over from a previous scenario is
// attributed to this one.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	samples = nil
}
