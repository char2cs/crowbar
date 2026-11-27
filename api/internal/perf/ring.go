package perf

import (
	"sync"
	"sync/atomic"
	"time"
)

const Cap = 4096

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

func SetEnabled(
	v bool,
) {
	enabled.Store(v)
}

func Enabled() bool {
	return enabled.Load()
}

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

func Snapshot() []Sample {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Sample, len(samples))
	copy(out, samples)
	return out
}

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	samples = nil
}
