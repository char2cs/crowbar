//go:build integration

package agent_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestAgent_LiveClaudeRecordsEveryMessageOfATurn is the user's own reproduction,
// driven against a REAL claude CLI through the real hook socket.
//
// They asked claude to "send me a message, wait 30 seconds, then send another".
// claude did exactly that, and Crowbar recorded ONE assistant turn: the second
// message. The first was never ingested, because the only thing any hook carries
// is `last_assistant_message` — literally the last one.
//
// A fixture test cannot stand in for this, and that is the whole reason this file
// exists: the defect was that the SHAPE OF REAL TRAFFIC differs from what the
// hook payloads implied. What has to be proven live is that claude really does
// write a mid-turn message into its transcript before it fires the tool hook that
// lets Crowbar look — a timing nobody can assert from a hand-written fixture.
//
// The prompt is deliberately mechanical. It asks for two marked lines with a slow
// tool call between them, so the assertion is on two exact strings rather than on
// anything a model had to be persuaded to say.
func TestAgent_LiveClaudeRecordsEveryMessageOfATurn(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-transcript", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID, "claude never bound a session: %+v", runner)

	// The script is deliberately impossible to satisfy WITHOUT the tool call: the
	// closing message has to quote a word only the command can produce. An earlier
	// version merely asked for a sleep in between, and a model that decided the
	// sleep was pointless answered with the first marker alone and ended the turn
	// in four seconds — a flaky PROMPT, which reads exactly like a flaky fix.
	const (
		first  = "MARKER-ONE-6F2A"
		second = "MARKER-TWO-"
		secret = "SECRET-4B7C"
	)
	drive(t, h, tap, termSessID,
		"You are being driven by an automated harness. Follow this script exactly, in order. "+
			"(1) Your first message must be exactly this line and nothing else: "+first+" "+
			"(2) Then call the Bash tool with exactly this command: sleep 12 && echo "+secret+" "+
			"(3) Your final message must be exactly the line "+second+
			" immediately followed by the word that command printed. "+
			"Do not skip the Bash call and do not answer step 3 from memory.")

	awaitTurnComplete(t, h, wsID, chatID, "claude")

	// DUMP_SCREEN=1 prints the CLI's own screen and the whole recorded conversation.
	// It is here because this is a LIVE test driven by a model: when it fails, the
	// only question worth asking first is whether the model followed the script at
	// all, and that is answerable from the screen and unanswerable from the
	// assertion. Off by default; it costs a getenv.
	if os.Getenv("DUMP_SCREEN") != "" {
		t.Logf("PTY SCREEN:\n%s", tap.Screen())
		for i, tn := range readLedgerTurns(t, h, wsID, chatID) {
			t.Logf("RECORD[%d] role=%s provider=%s text=%.400q", i, tn.Role, tn.Provider, tn.Text)
		}
	}
	replies := assistantReplies(readLedgerTurns(t, h, wsID, chatID), "claude")
	t.Logf("claude produced %d assistant messages in one turn:", len(replies))
	for i, r := range replies {
		t.Logf("  [%d] %q", i, r)
	}

	// The defect, stated as the assertion that used to fail: the FIRST message the
	// agent said has to be in the record at all.
	require.GreaterOrEqual(t, len(replies), 2,
		"a turn that produced two messages must be recorded as two: before the fix this was 1, "+
			"holding only the last message the Stop hook carried")

	firstAt, secondAt := -1, -1
	for i, r := range replies {
		if firstAt < 0 && strings.Contains(r, first) {
			firstAt = i
		}
		if strings.Contains(r, second+secret) {
			secondAt = i
		}
	}
	require.GreaterOrEqual(t, firstAt, 0,
		"the message claude said BEFORE its slow work is the one that used to be thrown away")
	require.GreaterOrEqual(t, secondAt, 0, "the terminating message must still be recorded")
	assert.Less(t, firstAt, secondAt,
		"the two messages must be recorded in the order the agent said them")

	// Nothing was said twice. The two sources — the transcript and the terminating
	// hook — describe one final message between them, and recording it from both
	// would be the opposite failure.
	assert.Equal(t, 1, countContaining(replies, second+secret),
		"the final message must appear exactly once, from whichever source got there first")
	assert.Equal(t, 1, countContaining(replies, first),
		"the mid-turn message must appear exactly once")

	// And every tool call is still attached to a turn that EXISTS in the record.
	// Splitting a turn at each message must not orphan the activity that hung off
	// it — that association is what lets the chat say which work produced which
	// reply.
	activity, err := h.app.Usecases.Agent.ReadActivity(context.Background(), chatID, 0, 0)
	require.NoError(t, err)
	turnIDs := map[string]bool{}
	for _, m := range readMessageTurnIDs(t, h, chatID) {
		turnIDs[m] = true
	}
	require.NotEmpty(t, activity.ToolCalls,
		"this test's premise is a turn with WORK between two messages; a turn that ran no tool at all "+
			"means the model did not follow the script, not that the record is wrong")
	for _, call := range activity.ToolCalls {
		assert.Truef(t, turnIDs[call.TurnID],
			"tool call %q (%s) is attached to turn %q, which is not any recorded message",
			call.ID, call.Name, call.TurnID)
	}
}

func countContaining(replies []string, needle string) int {
	n := 0
	for _, r := range replies {
		if strings.Contains(r, needle) {
			n++
		}
	}
	return n
}

// readMessageTurnIDs returns the turn id of every ASSISTANT message in the
// record, which is the set a tool call is allowed to be attached to.
func readMessageTurnIDs(t *testing.T, h *harness, chatID string) []string {
	t.Helper()
	page, err := h.app.Usecases.Agent.ReadMessages(context.Background(), chatID, 0, 0, 200)
	require.NoError(t, err)
	var out []string
	for _, m := range page.Items {
		if m.Role == "assistant" {
			out = append(out, m.ID)
		}
	}
	return out
}
