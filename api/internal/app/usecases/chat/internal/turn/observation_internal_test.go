package turn

import (
	"testing"

	"github.com/stretchr/testify/assert"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func compactionEvent(turnID string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{
		TurnID:    turnID,
		Interrupt: &engineagents.InterruptEvent{Kind: engineagents.InterruptCompaction},
	}
}

// TestRegression_SeparateCompactionsGetDistinctInterruptionIDs guards the bug
// reported live: a chat's SECOND compaction (manual, after an earlier
// automatic one) silently overwrote the first one's whole interruption row —
// transcript position and manual/automatic label both — because every
// compaction a chat ever has shared the identical
// "interrupt-"+chatID+"-compaction" id, and SaveInterruption upserts by that
// id. Two rounds with different ev.TurnID (codex maps a fresh turnId per
// compact_pre/compact_post pair — see codex.yaml) must now mint different
// ids.
func TestRegression_SeparateCompactionsGetDistinctInterruptionIDs(t *testing.T) {
	first := interruptionID(t.Context(), "chat1", compactionEvent("turn-1"))
	second := interruptionID(t.Context(), "chat1", compactionEvent("turn-2"))

	assert.NotEqual(t, first, second,
		"two separate compaction rounds must not collide onto the same interruption row")
}

// TestRegression_SameCompactionRoundKeepsOneInterruptionID guards the other
// half: compact_pre and compact_post of the SAME round share one ev.TurnID
// (both map turn_id from the same wrapping turn/started..turn/completed
// envelope — codex.yaml), and compact_post's ResolveInterruption call must
// still land on the row compact_pre's Interrupt call opened.
func TestRegression_SameCompactionRoundKeepsOneInterruptionID(t *testing.T) {
	pre := interruptionID(t.Context(), "chat1", compactionEvent("turn-1"))
	post := interruptionID(t.Context(), "chat1", compactionEvent("turn-1"))

	assert.Equal(t, pre, post,
		"compact_pre and compact_post of the same round must agree on one id so post resolves what pre opened")
}

// TestInterruptionID_CompactionWithNoTurnIDFallsBackToTheFixedShape guards
// claude's own compaction path: claude.yaml's PreCompact/PostCompact map no
// turn_id at all, so ev.TurnID is always empty for it. That must keep the
// pre-fix fixed shape rather than growing a trailing "-" with nothing after
// it, and pre/post must still agree with each other (claude has one
// compaction in flight at a time, same as before this fix).
func TestInterruptionID_CompactionWithNoTurnIDFallsBackToTheFixedShape(t *testing.T) {
	id := interruptionID(t.Context(), "chat1", compactionEvent(""))

	assert.Equal(t, "interrupt-chat1-compaction", id)
}
