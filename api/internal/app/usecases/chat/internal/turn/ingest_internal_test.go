package turn

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// fakeLiveConn reports a fixed HasLiveAPIConnection answer for every runner —
// apiOwnsThisEvent is the only thing under test here, so nothing else on
// Runners needs a real implementation.
type fakeLiveConn struct {
	Runners
	live bool
}

func (f fakeLiveConn) HasLiveAPIConnection(string) bool { return f.live }

func descriptorFor(t *testing.T, provider string) engineagents.Agent {
	t.Helper()
	agent, err := engineagents.New().Get(t.Context(), t.TempDir(), provider)
	require.NoError(t, err)
	return agent
}

// TestApiOwnsThisEvent_ guards the mechanism behind the bug reported live
// 2026-08-28: while working with codex, some turns went missing mid-stream and
// then all reappeared at once when the turn finished. Root cause, confirmed
// against the live daemon's own process list: every api-transport spawn ALSO
// forks a real, hooks-wired companion PTY on the SAME session (attach.go's own
// "known gap"), and spawn.Inject applies the descriptor's FULL hook set to it
// regardless of what TransportFor declares — that distinction is invisible to
// the actual CLI process, which just fires whatever hooks it is configured
// with. codex.yaml declares turn_stop and message_delta api-owned (no
// per-event override, so they inherit runtime.transport: api), which
// pumpAPIConn (apiconn.go) already reports over the live connection — so the
// companion PTY's hooks delivery of the SAME event is a redundant echo, not
// new information.
func TestApiOwnsThisEvent_APIOwnedEventOnALiveConnectionIsRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true}}
	codex := descriptorFor(t, "codex")

	require.True(t, turns.apiOwnsThisEvent(t.Context(), "runner-1", codex, "turn_stop"),
		"codex declares no per-event transport for turn_stop, so it inherits runtime.transport: api")
}

func TestApiOwnsThisEvent_APIOwnedEventWithNoLiveConnectionIsNotRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: false}}
	codex := descriptorFor(t, "codex")

	require.False(t, turns.apiOwnsThisEvent(t.Context(), "runner-1", codex, "turn_stop"),
		"with no live api connection there is no OTHER copy for a hooks delivery to be redundant with")
}

func TestApiOwnsThisEvent_AnEventExplicitlyDeclaredHooksOwnedIsNeverRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true}}
	codex := descriptorFor(t, "codex")

	require.False(t, turns.apiOwnsThisEvent(t.Context(), "runner-1", codex, "subagent_pre"),
		"codex.yaml overrides subagent_pre to transport: hooks precisely because the api transport never carries it")
}

func TestApiOwnsThisEvent_AHooksOnlyProviderNeverConsidersAnythingRedundant(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true}}
	claude := descriptorFor(t, "claude")

	require.False(t, turns.apiOwnsThisEvent(t.Context(), "runner-1", claude, "turn_stop"),
		"claude declares no api transport at all (runtime.transport: hooks) — HasLiveAPIConnection is a lie this test forces, and TransportFor is what must still say no")
}

// TestRegression_TheAPITransportDeliveryItselfIsNeverTreatedAsARedundantEcho is
// the bug reported live 2026-08-29: "Codex still not worky" — a fresh codex
// chat's very first prompt got a real reply over the wire (confirmed via the
// daemon's own trace: session_start through turn_stop all resolved and were
// pushed onto the api driver's Events() channel), yet the chat never went
// Working and the ledger never gained a single message.
//
// Root cause: descriptor.TransportFor(canonical) == "api" && a live connection
// exists are BOTH true for the api-transport delivery ITSELF, not just for the
// companion PTY's redundant hooks echo of it — the two facts this function
// used to gate on cannot tell the deliveries apart, only their ORIGIN can
// (inflight.FromAPITransport, set by pumpAPIConn on every event it forwards).
// Before that marker existed, this call — pumpAPIConn's own — satisfied the
// exact same "redundant, drop it" condition the tests above correctly want
// for the OTHER copy, and dropped session_start through turn_stop right along
// with it.
func TestRegression_TheAPITransportDeliveryItselfIsNeverTreatedAsARedundantEcho(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: true}}
	codex := descriptorFor(t, "codex")

	ctx := inflight.WithAPITransport(t.Context())
	require.False(t, turns.apiOwnsThisEvent(ctx, "runner-1", codex, "turn_stop"),
		"THE FIX: this call IS the api-transport delivery — never the redundant hooks copy it would otherwise look identical to")
}
