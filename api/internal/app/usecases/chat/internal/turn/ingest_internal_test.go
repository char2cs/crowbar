package turn

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The delivery's channel — never the event's static declaration — picks the
// descriptor block a payload is parsed with.
func TestChannelFor_APITransportCtxSelectsTheAPIChannel(t *testing.T) {
	assert.Equal(t, engineagents.ChannelAPI, channelFor(inflight.WithAPITransport(context.Background())))
}

func TestChannelFor_APlainCtxSelectsTheHooksChannel(t *testing.T) {
	assert.Equal(t, engineagents.ChannelHooks, channelFor(context.Background()))
}

// fakeLiveConn reports a fixed connection liveness and originated record for
// every runner; nothing else on Runners is implemented.
type fakeLiveConn struct {
	Runners
	live       bool
	originated map[string]bool
}

func (fakeLiveConn) ConfirmLaunch(string) {}

func (f fakeLiveConn) HasLiveAPIConnection(string) bool { return f.live }

// originated is whatever this connection minted itself. Empty is the honest
// answer for live:false — a connection that is gone keeps no record.
func (f fakeLiveConn) OriginatedSession(_, sessionID string) bool {
	return f.originated[sessionID]
}

func descriptorFor(t *testing.T, provider string) engineagents.Agent {
	t.Helper()
	agent, err := engineagents.New().Get(t.Context(), t.TempDir(), provider)
	require.NoError(t, err)
	return agent
}

// showingRunners answers only ShowingNativeView — surfaceGated is the only
// thing under test below, so nothing else on Runners needs a real
// implementation.
type showingRunners struct {
	Runners
	showing bool
}

func (showingRunners) ConfirmLaunch(string) {}

func (r showingRunners) ShowingNativeView(string) bool { return r.showing }

// TestSurfaceGated_AnEventWithNoDeclaredSurfacesIsNeverGated proves the
// default direction (design spec P6b tag 2): absence means every surface, so
// nothing changes for the vast majority of events that declare none — this
// must hold WITHOUT ever consulting ShowingNativeView, which showingRunners
// here would happily answer true for if asked.
func TestSurfaceGated_AnEventWithNoDeclaredSurfacesIsNeverGated(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: showingRunners{showing: true}}
	codex := descriptorFor(t, "codex")

	assert.False(t, turns.surfaceGated("runner-1", codex, "turn_stop"),
		"turn_stop declares no surfaces: — absent means every surface")
}

// TestSurfaceGated_AChatOnlyEventIsGatedOffWhileTheNativeViewIsShowing is the
// mechanism design spec P6b tag 2 exists for: codex.yaml declares
// message_delta surfaces: [chat] (it is live-only text with no durable
// write, purely for Crowbar's own chat surface) — while the user is looking
// at codex's own terminal (ShowingNativeView true), this delivery is not
// worth listening to.
func TestSurfaceGated_AChatOnlyEventIsGatedOffWhileTheNativeViewIsShowing(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: showingRunners{showing: true}}
	codex := descriptorFor(t, "codex")

	assert.True(t, turns.surfaceGated("runner-1", codex, "message_delta"),
		"message_delta is gated to chat: and the native terminal is on screen")
}

func TestSurfaceGated_AChatOnlyEventFlowsWhenTheNativeViewIsNotShowing(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: showingRunners{showing: false}}
	codex := descriptorFor(t, "codex")

	assert.False(t, turns.surfaceGated("runner-1", codex, "message_delta"),
		"chat IS the surface in front of the user, so a chat-gated event must flow")
}
