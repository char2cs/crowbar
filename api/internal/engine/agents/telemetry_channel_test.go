package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestRegression_CodexReportsNoUsageOnItsHooksChannel(t *testing.T) {
	codex := get(t, "codex")

	assert.Equal(t, string(agents.ChannelAPI), codex.TelemetryChannel())
	assert.True(t, codex.TelemetryOnSurface(agents.SurfaceChat))
	assert.False(t, codex.TelemetryOnSurface(agents.SurfaceTerminal),
		"no recorded codex hooks payload carries usage; the gauge must be absent, never stale")
}

func TestTelemetryOnSurface_ClaudeReportsOnBothOfItsSurfaces(t *testing.T) {
	claude := get(t, "claude")

	assert.Equal(t, string(agents.ChannelHooks), claude.TelemetryChannel())
	assert.True(t, claude.TelemetryOnSurface(agents.SurfaceChat))
	assert.True(t, claude.TelemetryOnSurface(agents.SurfaceTerminal))
}

func TestTelemetryOnSurface_TheDefaultLandingIsNotADecision(t *testing.T) {
	assert.True(t, get(t, "codex").TelemetryOnSurface(""))
	assert.True(t, get(t, "claude").TelemetryOnSurface(""))
}
