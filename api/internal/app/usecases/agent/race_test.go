package agent_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIngestHook_ConcurrentSessionStarts_NeverProduceTwoActiveSegmentsForSameCrowbarSegID
// is the regression test for the ingest-not-atomic-with-reducer bug: IngestHook's
// read (GetActiveSegmentByCrowbarID) -> reduce (Registry.OnSessionStart) ->
// persist sequence was not atomic as a whole (the Registry's own mutex only
// serializes the in-memory decision), so two concurrent session_start hooks
// for the SAME crowbarSegID could each read the same "active" segment
// snapshot before either persisted, and each independently create a new
// "active" segment — violating the documented
// ≤1-active-segment-per-CrowbarSegmentID invariant (agentchat.Store).
//
// Run with -race: this also exercises the segmentMutex/serialize path for
// data races, not just the logical invariant.
func TestIngestHook_ConcurrentSessionStarts_NeverProduceTwoActiveSegmentsForSameCrowbarSegID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	// Seed an initial bound session so every session_start fired below is a
	// genuine "registered" (unknown-id) move, not the special first-ever
	// "bound" case (which never creates a second active segment on its own).
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-seed"})))

	const n = 25
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		// Payload is marshaled here, on the main test goroutine, not inside the
		// spawned goroutine below: t.Helper()/require calls are only safe from
		// the goroutine running the Test function.
		payload := mustJSON(t, map[string]any{
			"session_id": fmt.Sprintf("sid-concurrent-%d", i),
		})
		go func(payload []byte) {
			defer wg.Done()
			_ = f.usecase.IngestHook(ctx, segID, "claude", "session_start", payload)
		}(payload)
	}
	wg.Wait()

	all, err := f.repo.AllSegments(ctx)
	require.NoError(t, err)

	var active int
	for _, s := range all {
		if s.CrowbarSegmentID == segID && s.Status == "active" {
			active++
		}
	}
	assert.Equal(t, 1, active,
		"at most one active segment must exist per CrowbarSegmentID even under a storm of concurrent session_start hooks; segments=%+v", all)
}
