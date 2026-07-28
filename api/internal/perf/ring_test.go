package perf_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/perf"
)

func TestRecord_DisabledByDefault(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(false)
	perf.Record("git.diff", 5*time.Millisecond)
	assert.Empty(t, perf.Snapshot())
}

func TestRecord_CapturesNameAndDuration(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	perf.Record("git.diff", 5*time.Millisecond)

	got := perf.Snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, "git.diff", got[0].Name)
	assert.InDelta(t, 5.0, got[0].DurationMS, 0.001)
}

func TestSnapshot_EvictsOldestBeyondCap(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	for range perf.Cap + 100 {
		perf.Record("n", time.Millisecond)
	}

	assert.Len(t, perf.Snapshot(), perf.Cap)
}

func TestRecord_ConcurrentIsRaceFree(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				perf.Record("concurrent", time.Microsecond)
			}
		}()
	}
	wg.Wait()

	assert.Len(t, perf.Snapshot(), perf.Cap)
}

func TestSnapshot_ReturnsCopy(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false) })

	perf.Record("a", time.Millisecond)
	got := perf.Snapshot()
	got[0].Name = "mutated"

	assert.Equal(t, "a", perf.Snapshot()[0].Name)
}
