package turn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
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

// surfaceRunners answers only the one question holdForAnswer asks of the runner
// lifecycle: is this chat looking at its provider's own view right now.
type surfaceRunners struct {
	Runners

	native bool
}

func (r surfaceRunners) ShowingNativeView(string) bool { return r.native }

// answerableAgent declares every prompt answerable from Crowbar, which is what
// leaves "did the desk park this relay" as the only variable below.
type answerableAgent struct {
	engineagents.Agent
}

func (answerableAgent) AnswerCapability(string) (engineagents.AnswerCapability, bool) {
	return engineagents.AnswerCapability{Wait: time.Minute, Keys: []string{"allow", "deny"}}, true
}

func heldForAnswer(t *testing.T, native bool) bool {
	t.Helper()

	desk := answerdesk.New(answerdesk.DefaultRetention, nil)
	turns := New(Deps{Answers: desk})
	turns.SetRunners(surfaceRunners{native: native})

	const deliveryID = "delivery-1"
	turns.holdForAnswer(
		inflight.WithDeliveryID(t.Context(), deliveryID),
		domain.Chat{ID: "chat-1"},
		engineagents.Runner{ID: "runner-1"},
		answerableAgent{},
		engineagents.CanonicalEvent{Kind: engineagents.HookPermission},
		"choice-1",
		[]byte(`{"tool_name":"shell"}`),
	)

	_, held := desk.Pending(deliveryID)
	return held
}

// TestRegression_NativeViewLeavesThePromptToTheCLI pins the bug reported live
// against codex: with the chat handed over to the provider's own TUI, a
// permission still parked its hook relay on the answer desk. That relay IS the
// CLI's gate — codex sat on "Action Required" drawing nothing for the whole
// 270s budget, while the card holding the question renders only on the chat
// surface, which a non-hotswap provider cannot reach mid-turn.
func TestRegression_NativeViewLeavesThePromptToTheCLI(t *testing.T) {
	assert.False(t, heldForAnswer(t, true),
		"a chat showing its provider's own view must not have its CLI's gate held by Crowbar")
}

// The other half: with Crowbar's own chat in front of the user there is no
// other UI to defer to, so the relay is parked exactly as it always was.
func TestHoldForAnswer_ParksTheRelayWhenCrowbarIsTheSurface(t *testing.T) {
	assert.True(t, heldForAnswer(t, false),
		"a chat Crowbar is driving must still park the relay so its card can answer")
}
