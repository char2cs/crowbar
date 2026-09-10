package turn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
)

// An AUTOMATIC compaction fires inside the user's own turn, and codex maps
// turn_id from the wrapping envelope — which is that user turn. Arming on it
// armed the REAL turn's id, so the real turn's own turn/completed was swallowed
// as a compaction's and nothing closed the turn; the 5s provider-idle sweep
// abandoned it instead, recording the reply as a cut-off partial.
//
// Reproduced live against a real codex app-server with the context window
// lowered so an auto-compaction landed mid-turn: compact_pre and the single
// turn_stop carried the same turn id, and "closed a turn whose message was cut
// off" followed 5.2s later.
func TestRegression_AnAutomaticCompactionDoesNotSwallowTheRealTurnsStop(t *testing.T) {
	turns := New(Deps{InflightTurns: inflight.NewTurns()})
	turns.turns.Begin("runner-1", "chat-1")

	turns.armCompaction("chat-1", "turn-1")

	require.False(t, turns.compacting.consume("chat-1", "turn-1"),
		"the user turn's own stop must still close it, not be skipped as a compaction's")
}

// The standalone round trip — the compact button, no turn of the chat's own in
// flight — is what the latch exists for and must still be armed.
func TestRegression_AStandaloneCompactionIsStillArmed(t *testing.T) {
	turns := New(Deps{InflightTurns: inflight.NewTurns()})

	turns.armCompaction("chat-1", "turn-1")

	require.True(t, turns.compacting.consume("chat-1", "turn-1"),
		"a compaction that owns its whole turn envelope must still have that envelope's stop skipped")
}

// The whole reason this exists: measured against codex-cli 0.149.1,
// thread/compact/start's own turn/completed is byte-for-byte the same shape
// (items: [], itemsView: "notLoaded", status: "completed") as a genuinely
// interrupted real turn's, or a real turn that ran a tool and sent no final
// text. Only the turn id tells them apart.
func TestCompactionTurns_ArmedTurnIsConsumedOnce(t *testing.T) {
	c := newCompactionTurns()
	c.arm("chat1", "turn1")

	assert.True(t, c.consume("chat1", "turn1"), "the armed turn must match")
	assert.False(t, c.consume("chat1", "turn1"),
		"a turn only closes once — the second turn_stop for the same id is an ordinary one")
}

func TestCompactionTurns_AnUnrelatedTurnIDIsNotConsumed(t *testing.T) {
	c := newCompactionTurns()
	c.arm("chat1", "compaction-turn")

	assert.False(t, c.consume("chat1", "some-other-turn"),
		"an ordinary reply's own turn id must never be mistaken for the armed one")
	assert.True(t, c.consume("chat1", "compaction-turn"),
		"the real armed turn must still be there — the miss above must not have cleared it")
}

// A provider whose descriptor maps no turn_id at all (claude today) hands
// every canonical event an empty ev.TurnID. Two unrelated calls with an empty
// id must never agree with each other just because the map's zero value
// happens to match.
func TestCompactionTurns_BlankTurnIDNeverMatches(t *testing.T) {
	c := newCompactionTurns()
	c.arm("chat1", "") // arm() itself refuses a blank id, but assert consume() does too

	assert.False(t, c.consume("chat1", ""),
		"a blank turn id must never match, armed or not")
}

func TestCompactionTurns_ArmingAgainReplacesTheEarlierUnconsumedTurn(t *testing.T) {
	c := newCompactionTurns()
	c.arm("chat1", "turn1")
	c.arm("chat1", "turn2") // turn1's own close never arrived — a crashed compaction

	assert.False(t, c.consume("chat1", "turn1"),
		"the replaced id must no longer match, and the miss must not clear turn2")
	assert.True(t, c.consume("chat1", "turn2"), "turn2 must still be the one actually armed")
}

func TestCompactionTurns_IsScopedPerChat(t *testing.T) {
	c := newCompactionTurns()
	c.arm("chat1", "turn1")

	assert.False(t, c.consume("chat2", "turn1"),
		"the same turn id on a different chat must not match")
	assert.True(t, c.consume("chat1", "turn1"), "the real chat's own turn must still match")
}

// A Turns built field-by-field in a sibling test has no latch, and the
// turn-close paths call consume() unconditionally.
func TestCompactionTurns_NilIsSafe(t *testing.T) {
	var c *compactionTurns
	assert.NotPanics(t, func() {
		c.arm("chat1", "turn1")
		assert.False(t, c.consume("chat1", "turn1"))
	})
}
