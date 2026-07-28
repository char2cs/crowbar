// Package perf holds a bounded, race-free ring of timing samples used to
// attribute daemon-side latency during performance work.
//
// Crowbar's freezes are cross-boundary. A stalled interaction may be main-thread
// React work, HTTP round-trip time, a git subprocess, or contention on a repo
// lock — and a profiler attached to any one of those layers cannot say which of
// them owns the wall clock. Funnelling every layer's durations into one ordered
// ring, read back through a single endpoint, makes that split answerable without
// correlating four separate tools against four separate clocks.
//
// Recording is OFF by default, and costs one atomic load per Record call while
// disabled. That is precisely what lets the instrumented call sites — the git
// exec path, the repo mutex, every HTTP handler — stay in the tree permanently
// instead of being patched in for each investigation. They are the hot paths
// whose cost is under measurement, so an always-on ring would tax the very
// operations it exists to observe, and its mutex would serialise callers that
// today run concurrently. A capture run arms the ring, drives a scenario, reads
// it back, and disarms.
package perf

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cap bounds the ring. It is sized for a whole measurement scenario — minutes of
// interaction across hundreds of requests — rather than a few seconds, because a
// ring that wraps mid-run silently evicts the samples the capture is trying to
// read and reports a truncated window as if it were the whole story.
const Cap = 4096

// Sample is one timed operation. Name is the call site's bucket ("git.diff",
// "lock.read.hold", "http.GET /v0/workspaces/:wsId/review/files"), DurationMS
// its wall time, and At the instant it completed — ordering by At is what lets a
// reader nest subprocess samples underneath the request that spawned them.
//
// The duration is milliseconds rather than a time.Duration because the ring is
// read over JSON by tooling with no notion of Go's nanosecond encoding.
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
