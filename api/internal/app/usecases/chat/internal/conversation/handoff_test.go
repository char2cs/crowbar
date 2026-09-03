package conversation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// handoffCap discovers the live defaultChatLogTurns cap through the real
// RecentHandoffWindow rather than a copy of the number: a probe slice far
// larger than any plausible cap always comes back trimmed to exactly it, so a
// future change to the cap cannot leave this file silently testing the wrong
// boundary.
func handoffCap(t *testing.T) int {
	t.Helper()
	kept, _ := agenttools.RecentHandoffWindow("probe", make([]struct{}, 100_000))
	return len(kept)
}

// waitForClockTick blocks until real time.Now() has strictly advanced past its
// current instant — a real signal (the clock itself), never a guessed sleep
// duration. Needed wherever a gap boundary is a strict `>` against wall time
// (TurnsSince): two turns recorded back-to-back against the real sqlite-backed
// activity store can otherwise land on the identical instant, and the second
// is silently excluded from "since" rather than by RecentHandoffWindow's own
// cap — exactly the false positive a gap test here must not be exposed to.
func waitForClockTick(t *testing.T) {
	t.Helper()
	start := time.Now()
	for time.Now().Equal(start) {
		if time.Since(start) > 2*time.Second {
			t.Fatal("real clock never advanced")
		}
	}
}

// recordNumberedTurns appends n user turns with distinguishable text
// (prefix-0, prefix-1, ...) so a trimmed handoff can be checked for exactly
// which ones survived the cap.
func recordNumberedTurns(t *testing.T, f fixture, chat domain.Chat, prefix string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, f.conversations.RecordTurn(
			t.Context(), chat, "claude", "runner-1", "session-1", "user",
			fmt.Sprintf("%s-%d", prefix, i), "",
		))
	}
	f.settle()
}

// A gap under the cap must render exactly as it did before this cap existed:
// every turn present, no truncation note anywhere in the blob.
func TestAssembleConversation_UnderTheCapCarriesEveryTurnAndNoNote(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	under := handoffCap(t) - 1
	recordNumberedTurns(t, f, chat, "turn", under)

	blob, err := f.conversations.AssembleHandoff(t.Context(), chatID)

	require.NoError(t, err)
	assert.NotContains(t, blob, "get_chat_log", "an untrimmed handoff must carry no truncation note")
	for i := 0; i < under; i++ {
		assert.Contains(t, blob, fmt.Sprintf("turn-%d", i))
	}
}

// The fresh-join path (cut zero, the whole ledger) must trim the same way the
// resume path does: this pins the bug where a long-running chat on the OTHER
// provider dumped its entire history into the next provider's context.
func TestAssembleConversation_FreshJoinOverTheCapKeepsOnlyTheMostRecent(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	total := handoffCap(t) + 5
	recordNumberedTurns(t, f, chat, "turn", total)

	blob, err := f.conversations.AssembleConversation(t.Context(), chatID, false, time.Time{})

	require.NoError(t, err)
	assert.NotContains(t, blob, "turn-0\n", "the oldest turn must be trimmed from a fresh join")
	assert.Contains(t, blob, fmt.Sprintf("turn-%d", total-1), "the newest turn must survive")
	assert.Contains(t, blob, "get_chat_log", "a trimmed handoff must point at how to read the rest")
}

// The resume path caps the GAP itself, not just the ledger as a whole: turns
// from before the provider left never appear (unchanged, existing behaviour),
// and once the gap alone exceeds the cap, its own oldest turns are the ones
// trimmed.
func TestAssembleConversation_ResumeGapOverTheCapKeepsOnlyTheMostRecentOfTheGap(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)

	recordNumberedTurns(t, f, chat, "before", 3)
	turnsBefore, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	leftAt := turnsBefore[len(turnsBefore)-1].At

	// Without a real tick between them, the first gap turn below can land on
	// the identical wall-clock instant as leftAt and be silently excluded from
	// the gap by TurnsSince's own strict `>` filter — see waitForClockTick.
	waitForClockTick(t)

	total := handoffCap(t) + 5
	recordNumberedTurns(t, f, chat, "gap", total)

	blob, err := f.conversations.AssembleConversation(t.Context(), chatID, true, leftAt)

	require.NoError(t, err)
	assert.NotContains(t, blob, "before-", "turns from before the gap must never appear in a resume handoff")
	assert.NotContains(t, blob, "gap-0\n",
		"the oldest turn OF THE GAP must be trimmed once the gap itself exceeds the cap")
	assert.Contains(t, blob, fmt.Sprintf("gap-%d", total-1), "the newest gap turn must survive")
	assert.Contains(t, blob, "get_chat_log")
}
