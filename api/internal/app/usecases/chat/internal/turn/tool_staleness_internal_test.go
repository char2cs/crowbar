package turn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_StaleToolCall_ClosedAfterTheCeiling pins the live wedge
// measured on claude chat 7745f69f (2026-09-23). The Stop hook DID arrive and
// closed the turn cleanly at 19:50:17.302 (working=false). Two seconds later a
// PreToolUse for ScheduleWakeup arrived, tool_pre's restateAsyncWork relit the
// chat off it (asyncWork=1, still no turn open), and the matching PostToolUse
// never came — claude fires none on that path. From there nothing could ever
// recover the chat: claude declares not_emitted: [... idle], so the provider-idle
// fuse cannot fire, and abandonedMessage is gated on OpenWork, which this one
// running row held true forever.
//
// OpenWork already ran its subagents through withStaleSubagentsClosed and its
// tool calls through nothing at all — the asymmetry IS the bug. This is the
// tool-call half of that same read-only dead-letter net.
func TestRegression_StaleToolCall_ClosedAfterTheCeiling(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	stale := domain.ActivityToolCall{
		ID:        "toolu_01FeKyxrB8sfmuTiHZYUj2VA",
		Name:      "ScheduleWakeup",
		Status:    domain.ToolStatusRunning,
		StartedAt: now.Add(-3 * time.Hour),
	}

	out := withStaleToolCallsClosed([]domain.ActivityToolCall{stale}, now)

	require.Len(t, out, 1)
	assert.Equal(t, domain.ToolStatusAbandoned, out[0].Status,
		"a tool call running past the ceiling must read as abandoned, not as live work")
	require.NotNil(t, out[0].EndedAt)
	assert.True(t, out[0].EndedAt.Equal(now))
}

// A tool call well within the ceiling is the ordinary case — a long build, a
// codex `wait` sitting on a child — and must read exactly as it did before.
func TestToolStaleness_WithinTheCeilingStaysRunning(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	running := domain.ActivityToolCall{
		ID:        "tool-1",
		Status:    domain.ToolStatusRunning,
		StartedAt: now.Add(-5 * time.Minute),
	}

	out := withStaleToolCallsClosed([]domain.ActivityToolCall{running}, now)

	require.Len(t, out, 1)
	assert.Equal(t, domain.ToolStatusRunning, out[0].Status,
		"a tool call well inside the ceiling must stay running")
	assert.Nil(t, out[0].EndedAt)
}

// A call that already finished is never touched, however old — this helper only
// ever ADDS an ending to a row still claiming to run.
func TestToolStaleness_NeverRewritesAFinishedCall(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	realEnd := now.Add(-4 * time.Hour)
	done := domain.ActivityToolCall{
		ID:        "tool-1",
		Status:    domain.ToolStatusOK,
		StartedAt: now.Add(-5 * time.Hour),
		EndedAt:   &realEnd,
	}

	out := withStaleToolCallsClosed([]domain.ActivityToolCall{done}, now)

	require.Len(t, out, 1)
	assert.Equal(t, domain.ToolStatusOK, out[0].Status)
	require.NotNil(t, out[0].EndedAt)
	assert.True(t, out[0].EndedAt.Equal(realEnd), "must keep the real EndedAt, not the read time")
}

// A running call with no StartedAt has an age nothing can measure. Reading the
// zero time as "since year one" would abandon it on sight, which is the
// opposite of the conservative default this net is meant to be.
func TestToolStaleness_AZeroStartedAtIsNeverStale(t *testing.T) {
	now := time.Unix(10_000_000, 0)
	unknown := domain.ActivityToolCall{ID: "tool-1", Status: domain.ToolStatusRunning}

	out := withStaleToolCallsClosed([]domain.ActivityToolCall{unknown}, now)

	require.Len(t, out, 1)
	assert.Equal(t, domain.ToolStatusRunning, out[0].Status,
		"a call whose age cannot be measured must never be called stale")
	assert.Nil(t, out[0].EndedAt)
}
