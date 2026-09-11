package turn

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// A compaction's own turn_stop is skipped whole (compaction.go) — but the idle
// report riding with it is about the COMPACTION, not the assistant turn still
// running underneath it. Measured live against codex-cli: idle lands in the
// same millisecond as every turn/completed, so a compaction arms the latch as
// surely as a real close does.
//
// Left armed, the 5s provider-idle sweep (runner/internal/termwait,
// providerSaysItIsIdle) abandons that live turn — Working=false, spinner dark,
// while the CLI is still generating. It is deliberately NOT gated on OpenWork,
// so a running tool call does not save it.
//
// Only codex reaches this: it is the only descriptor mapping idle, and it marks
// no message final, so no mid-turn recordAssistantMessage ever re-opens the
// assistant turn to clear the latch on its own.
func TestRegression_ACompactionsOwnStopStillClearsTheIdleLatch(t *testing.T) {
	turns := New(Deps{})
	turns.compacting.arm("chat-1", "compaction-turn")
	turns.recordIdle(domain.Chat{ID: "chat-1"})

	_, armed := turns.ProviderIdleSince("chat-1")
	require.True(t, armed, "the idle report that rides the compaction's stop arms the latch")

	err := turns.closeTurnFromStop(
		t.Context(),
		domain.Chat{ID: "chat-1"},
		engineagents.Runner{},
		nil,
		engineagents.CanonicalEvent{Kind: "turn_stop", TurnID: "compaction-turn"},
	)
	require.NoError(t, err)

	_, stillArmed := turns.ProviderIdleSince("chat-1")
	require.False(t, stillArmed,
		"a compaction says nothing about the turn running underneath it; leaving the latch armed lets the 5s sweep abandon a live turn")
}

// The failure half of the same sum type takes the identical early return.
func TestRegression_ACompactionsOwnFailureStillClearsTheIdleLatch(t *testing.T) {
	turns := New(Deps{})
	turns.compacting.arm("chat-1", "compaction-turn")
	turns.recordIdle(domain.Chat{ID: "chat-1"})

	err := turns.closeTurnFromFailure(
		t.Context(),
		domain.Chat{ID: "chat-1"},
		engineagents.Runner{},
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed, TurnID: "compaction-turn"},
	)
	require.NoError(t, err)

	_, stillArmed := turns.ProviderIdleSince("chat-1")
	require.False(t, stillArmed, "a failed compaction leaves the same stale latch behind")
}
