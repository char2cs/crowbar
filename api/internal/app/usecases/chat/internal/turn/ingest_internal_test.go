package turn

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TestChannelFor_ pins the ONLY fact that tells ParseHook which channel-
// scoped block a delivery must be read through: whether pumpAPIConn marked
// this ctx (inflight.WithAPITransport) before calling IngestHook. An HTTP
// hook relay POST marks nothing, so the zero value is hooks — see
// ownerDropsThisDelivery's own doc comment for why only this ORIGIN, never
// an event's static Transport, can tell the two deliveries apart.
func TestChannelFor_APITransportCtxSelectsTheAPIChannel(t *testing.T) {
	assert.Equal(t, engineagents.ChannelAPI, channelFor(inflight.WithAPITransport(context.Background())))
}

func TestChannelFor_APlainCtxSelectsTheHooksChannel(t *testing.T) {
	assert.Equal(t, engineagents.ChannelHooks, channelFor(context.Background()))
}

// fakeLiveConn reports fixed HasLiveAPIConnection/HasDispatchedOverAPI answers
// for every runner — ownerDropsThisDelivery is the only thing under test
// here, so nothing else on Runners needs a real implementation. dispatched
// defaults to matching live (the ordinary shape: a connection that is live
// because something was already pushed down it), overridable per test.
type fakeLiveConn struct {
	Runners
	live       bool
	dispatched bool
	originated map[string]bool
}

func (f fakeLiveConn) HasLiveAPIConnection(string) bool { return f.live }
func (f fakeLiveConn) HasDispatchedOverAPI(string) bool { return f.dispatched }

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

// TestOwnerDropsThisDelivery_ guards the mechanism behind the bug reported
// live 2026-08-28: while working with codex, some turns went missing
// mid-stream and then all reappeared at once when the turn finished. Root
// cause, confirmed against the live daemon's own process list: every
// api-transport spawn ALSO forks a real, hooks-wired companion PTY on the
// SAME session (attach.go's own "known gap"), and spawn.Inject applies the
// descriptor's FULL hook set to it regardless of what TransportFor declares
// — that distinction is invisible to the actual CLI process, which just
// fires whatever hooks it is configured with. codex.yaml declares turn_stop
// owner: api (design spec P6b), which pumpAPIConn (apiconn.go) already
// reports over the live connection — so the companion PTY's hooks delivery
// of the SAME event is a redundant echo, not new information.
func TestOwnerDropsThisDelivery_APIOwnedEventOnALiveConnectionIsRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true, dispatched: true}}
	codex := descriptorFor(t, "codex")

	require.True(t, turns.ownerDropsThisDelivery(t.Context(), "runner-1", codex, "turn_stop"),
		"codex.yaml declares turn_stop owner: api, and this connection has actually carried a prompt")
}

// TestRegression_ALiveButUndispatchedConnectionNeverMakesTheCompanionPTYsHooksRedundant
// guards the bug reported live 2026-09-08: closing a codex tab mid-turn, or a
// prompt's replacement-spawn fallback (submitPromptOverAPI, prompts.go),
// establishes a live api connection that carries NOTHING of its own — the
// prompt actually rides the companion PTY's own resume+text argv. codex's
// title-setting MCP tool call (a synchronous, ledger-independent path) proved
// the CLI answered normally, yet the turn never appeared in the chat's
// message ledger at all: HasLiveAPIConnection alone made this function treat
// the companion PTY's real hooks as a redundant echo of a report the api side
// was never asked to make.
//
// THE ZERO-WRITER CASE (design spec P6b): declaring an owner says who is
// AUTHORITATIVE, never whether that owner actually carried THIS turn. This
// is what makes the drop below impossible: owner: api names the OTHER
// channel, that channel IS live, but nothing has been dispatched over it, so
// there is still no api-side report for this hooks delivery to be redundant
// with — dropping it here would leave the turn with zero writers.
func TestRegression_ALiveButUndispatchedConnectionNeverMakesTheCompanionPTYsHooksRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true, dispatched: false}}
	codex := descriptorFor(t, "codex")

	require.False(t, turns.ownerDropsThisDelivery(t.Context(), "runner-1", codex, "turn_stop"),
		"the connection is live but has dispatched nothing, so it has nothing of its own for this hooks delivery to be a redundant copy of")
}

func TestOwnerDropsThisDelivery_APIOwnedEventWithNoLiveConnectionIsNotRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: false}}
	codex := descriptorFor(t, "codex")

	require.False(t, turns.ownerDropsThisDelivery(t.Context(), "runner-1", codex, "turn_stop"),
		"with no live api connection there is no OTHER copy for a hooks delivery to be redundant with")
}

// TestRegression_EveryDualShapeCodexEventIsRedundantOnALiveDispatchedConnection
// sweeps every codex.yaml event sharing turn_stop's own dual-shape hazard —
// session_start, user_prompt, tool_pre, tool_post, permission, compact_pre
// and compact_post all declare owner: api (design spec P6b) AND are also
// fired hooks-shaped, unconditionally, by codex's own config.toml (see
// codex.yaml's config_injection hooks.* entries). ownerDropsThisDelivery
// reads only the descriptor's own owner: declaration — no event-name
// branching — so the "drop the companion PTY's redundant echo" guard the bug
// report above exercised through turn_stop must hold for every one of them.
func TestRegression_EveryDualShapeCodexEventIsRedundantOnALiveDispatchedConnection(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true, dispatched: true}}
	codex := descriptorFor(t, "codex")

	for _, event := range []string{
		"session_start", "user_prompt", "turn_stop",
		"tool_pre", "tool_post", "permission", "compact_pre", "compact_post",
	} {
		t.Run(event, func(t *testing.T) {
			require.True(t, turns.ownerDropsThisDelivery(t.Context(), "runner-1", codex, event),
				"codex.yaml declares owner: api for this event; a live, dispatched api connection "+
					"already reports it, so the companion PTY's hooks copy must be treated as redundant")
		})
	}
}

func TestOwnerDropsThisDelivery_AnEventWithNoDeclaredOwnerIsNeverRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true, dispatched: true}}
	codex := descriptorFor(t, "codex")

	require.False(t, turns.ownerDropsThisDelivery(t.Context(), "runner-1", codex, "subagent_pre"),
		"subagent_pre declares no api: block at all, so it has no owner: to name the api channel — "+
			"absent means either, and either is never dropped")
}

func TestOwnerDropsThisDelivery_AHooksOnlyProviderNeverConsidersAnythingRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true}}
	claude := descriptorFor(t, "claude")

	require.False(t, turns.ownerDropsThisDelivery(t.Context(), "runner-1", claude, "turn_stop"),
		"claude declares no dual-channel events at all — HasLiveAPIConnection is a lie this test "+
			"forces, and EventOwner's absent-means-either default is what must still say no")
}

// TestRegression_TheAPITransportDeliveryItselfIsNeverTreatedAsARedundantEcho is
// the bug reported live 2026-08-29: "Codex still not worky" — a fresh codex
// chat's very first prompt got a real reply over the wire (confirmed via the
// daemon's own trace: session_start through turn_stop all resolved and were
// pushed onto the api driver's Events() channel), yet the chat never went
// Working and the ledger never gained a single message.
//
// Root cause: owner: api and a live connection existing are BOTH true for
// the api-transport delivery ITSELF, not just for the companion PTY's
// redundant hooks echo of it — the two facts this function gates on cannot
// tell the deliveries apart, only their ORIGIN can (inflight.
// FromAPITransport, set by pumpAPIConn on every event it forwards). Before
// that marker existed, this call — pumpAPIConn's own — satisfied the exact
// same "redundant, drop it" condition the tests above correctly want for the
// OTHER copy, and dropped session_start through turn_stop right along with
// it.
func TestRegression_TheAPITransportDeliveryItselfIsNeverTreatedAsARedundantEcho(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true}}
	codex := descriptorFor(t, "codex")

	ctx := inflight.WithAPITransport(t.Context())
	require.False(t, turns.ownerDropsThisDelivery(ctx, "runner-1", codex, "turn_stop"),
		"THE FIX: this call IS the api-transport delivery — never the redundant hooks copy it would otherwise look identical to")
}

// showingRunners answers only ShowingNativeView — surfaceGated is the only
// thing under test below, so nothing else on Runners needs a real
// implementation.
type showingRunners struct {
	Runners
	showing bool
}

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
