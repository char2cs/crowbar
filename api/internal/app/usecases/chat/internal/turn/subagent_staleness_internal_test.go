package turn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_StaleSubagent_ClosedAfterTheCeiling pins the live bug: a
// subagent whose own subagent_post is simply lost — dropped, or a
// background fork whose stop hook never fires — while its ENCLOSING turn
// finishes completely normally (no stall, no abandon) had no cleanup path
// at all before this fix. Both the chat's own working state and the
// subagent shelf spun forever off its never-set EndedAt.
func TestRegression_StaleSubagent_ClosedAfterTheCeiling(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	stale := domain.ActivitySubagent{
		ID:        "sub-1",
		StartedAt: now.Add(-3 * time.Hour),
	}

	out := withStaleSubagentsClosed([]domain.ActivitySubagent{stale}, now)

	require.Len(t, out, 1)
	require.NotNil(t, out[0].EndedAt, "a subagent open past the ceiling must read as ended")
	assert.True(t, out[0].EndedAt.Equal(now))
}

// A subagent well within the ceiling — the ordinary, legitimate case
// (TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking, turn_test.go)
// — must read exactly as it did before this fix: still open.
func TestSubagentStaleness_WithinTheCeilingStaysOpen(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	recent := domain.ActivitySubagent{
		ID:        "sub-1",
		StartedAt: now.Add(-5 * time.Minute),
	}

	out := withStaleSubagentsClosed([]domain.ActivitySubagent{recent}, now)

	require.Len(t, out, 1)
	assert.Nil(t, out[0].EndedAt, "a subagent well inside the ceiling must stay open")
}

// A subagent that already carries its own real EndedAt must never be
// touched, whatever it says — this helper only ever ADDS an ending, never
// overwrites one a real subagent_post already recorded.
func TestSubagentStaleness_NeverOverwritesARealEndedAt(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	realEnd := now.Add(-4 * time.Hour)
	ended := domain.ActivitySubagent{
		ID:        "sub-1",
		StartedAt: now.Add(-5 * time.Hour),
		EndedAt:   &realEnd,
	}

	out := withStaleSubagentsClosed([]domain.ActivitySubagent{ended}, now)

	require.Len(t, out, 1)
	require.NotNil(t, out[0].EndedAt)
	assert.True(t, out[0].EndedAt.Equal(realEnd), "must keep the real EndedAt, not the read time")
}
