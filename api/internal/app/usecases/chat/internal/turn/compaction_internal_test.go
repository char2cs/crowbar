package turn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
