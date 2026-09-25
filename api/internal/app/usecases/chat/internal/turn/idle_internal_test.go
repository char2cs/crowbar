package turn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
)

// The whole reason this is a latch and not an action: measured against codex-cli
// 0.149.1, thread/status/changed(idle) lands IMMEDIATELY BEFORE turn/completed on
// a perfectly healthy turn. A reconcile that fired on the report itself would
// abandon every turn microseconds before it ended properly.
func TestIdleLatch_AClosedTurnDisarmsIt(t *testing.T) {
	l := newIdleLatch()
	l.arm("c1", time.Unix(100, 0))
	require.True(t, armed(l, "c1"))

	l.clear("c1")

	_, ok := l.at("c1")
	assert.False(t, ok, "a turn that closed normally leaves nothing to reconcile")
}

// A provider that repeats itself must not keep pushing the deadline out, or a
// chatty idle report would hold the reconcile off forever.
func TestIdleLatch_KeepsTheEarliestReport(t *testing.T) {
	l := newIdleLatch()
	first := time.Unix(100, 0)
	l.arm("c1", first)
	l.arm("c1", first.Add(time.Minute))

	at, ok := l.at("c1")
	require.True(t, ok)
	assert.Equal(t, first, at)
}

func TestIdleLatch_IsScopedPerChat(t *testing.T) {
	l := newIdleLatch()
	l.arm("c1", time.Unix(100, 0))
	l.clear("c2")

	assert.True(t, armed(l, "c1"), "clearing one chat must not disarm another")
}

// A Turns built field-by-field in a sibling test has no latch, and the turn-close
// paths call clear() unconditionally.
func TestIdleLatch_NilIsSafe(t *testing.T) {
	var l *idleLatch
	assert.NotPanics(t, func() {
		l.arm("c1", time.Unix(100, 0))
		l.clear("c1")
		_, ok := l.at("c1")
		assert.False(t, ok)
	})
}

func armed(l *idleLatch, chatID string) bool {
	_, ok := l.at(chatID)
	return ok
}

// A7: erasing a chat leaves nothing the turn ingress held for it in memory.
func TestTurns_ForgetChatDropsEveryPerChatEntry(t *testing.T) {
	turns := &Turns{
		idle: newIdleLatch(), live: newLiveText(), messages: stream.New(),
		compacting: newCompactionTurns(), manualCompact: newManualCompactRequests(),
	}
	turns.idle.arm("c1", time.Unix(100, 0))
	turns.live.observe("c1", "reasoning", "b1", 0, "thinking")
	turns.messages.Observe("c1", "r1", "t1", "m1", 0, false, false, "partial", time.Unix(100, 0))
	turns.compacting.arm("c1", "turn-compact")
	turns.manualCompact.arm("c1")

	turns.ForgetChat("c1")

	_, idle := turns.idle.at("c1")
	_, live := turns.live.sinceLastDelta("c1")
	assert.False(t, idle)
	assert.False(t, live)
	assert.Empty(t, turns.messages.UnfinishedAcrossRunners("c1"))
	assert.False(t, turns.compacting.consume("c1", "turn-compact"))
	assert.False(t, turns.manualCompact.peek("c1"))
}
