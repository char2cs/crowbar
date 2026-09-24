package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TestRegression_CodexReportsNoUsageOnItsHooksChannel is a MEASUREMENT, not a
// preference, and it is the evidence behind making the context gauge absent
// on codex's terminal surface.
//
// Measured three ways, none of them inference:
//   - the 11 hooks codex's own config_injection wires are PermissionRequest,
//     PostCompact, PostToolUse, PreCompact, PreToolUse, SessionEnd,
//     SessionStart, Stop, SubagentStart, SubagentStop, UserPromptSubmit.
//     There is no status-line hook among them — claude's usage reports ride
//     exactly that, and codex declares no counterpart.
//   - the THREE recorded codex hooks payloads (protocol/testdata/fixtures/
//     codex/{SessionStart,UserPromptSubmit,Stop}.json — verbatim captures off
//     a real 0.146.0 process, per that directory's README) carry session_id,
//     turn_id, transcript_path, cwd, hook_event_name, model, permission_mode
//     and the event's own field. No token, usage, context or cost field
//     appears in any of them.
//   - token usage appears in exactly one recorded payload,
//     thread_tokenUsage_updated.json, an app-server notification.
//
// So codex's usage mechanism is api-channel and there is no hooks-side one to
// declare. A gauge on the terminal surface would be a number that can never
// move.
func TestRegression_CodexReportsNoUsageOnItsHooksChannel(t *testing.T) {
	codex := get(t, "codex")

	assert.Equal(t, string(agents.ChannelAPI), codex.TelemetryChannel())
	assert.True(t, codex.TelemetryOnSurface(agents.SurfaceChat))
	assert.False(t, codex.TelemetryOnSurface(agents.SurfaceTerminal),
		"no recorded codex hooks payload carries usage; the gauge must be absent, never stale")
}

// claude's usage reports ride its statusLine callback — a HOOKS mechanism —
// and both of its surfaces are hooks-channel, so the gauge is live on either.
// This is the case that proves the rule is about the CHANNEL and not about
// the terminal being second-class.
func TestTelemetryOnSurface_ClaudeReportsOnBothOfItsSurfaces(t *testing.T) {
	claude := get(t, "claude")

	assert.Equal(t, string(agents.ChannelHooks), claude.TelemetryChannel())
	assert.True(t, claude.TelemetryOnSurface(agents.SurfaceChat))
	assert.True(t, claude.TelemetryOnSurface(agents.SurfaceTerminal))
}

// The provider's own default landing declares no channel, and absence is not
// a decision: every chat that predates surfaces keeps the gauge it had.
func TestTelemetryOnSurface_TheDefaultLandingIsNotADecision(t *testing.T) {
	assert.True(t, get(t, "codex").TelemetryOnSurface(""))
	assert.True(t, get(t, "claude").TelemetryOnSurface(""))
}
