//go:build integration

package agent_test

import (
	"cmp"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestAgent_LiveClaudeRecordsEveryMessageOfATurn(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-transcript", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID, "claude never bound a session: %+v", runner)

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

	assert.Equal(t, 1, countContaining(replies, second+secret),
		"the final message must appear exactly once, from whichever source got there first")
	assert.Equal(t, 1, countContaining(replies, first),
		"the mid-turn message must appear exactly once")

	calls := toolRuns(t, h, chatID)
	turnIDs := map[string]bool{}
	for _, m := range readMessageTurnIDs(t, h, chatID) {
		turnIDs[m] = true
	}
	require.NotEmpty(t, calls,
		"this test's premise is a turn with WORK between two messages; a turn that ran no tool at all "+
			"means the model did not follow the script, not that the record is wrong")
	for _, call := range calls {
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

func readMessageTurnIDs(t *testing.T, h *harness, chatID string) []string {
	t.Helper()
	var out []string
	for _, m := range kit.Said(chatRows(t, h, chatID)) {
		if m.Role == "assistant" {
			out = append(out, m.TurnID)
		}
	}
	return out
}

// chatRows are chatID's ledger rows, as its feed carries them.
func chatRows(t *testing.T, h *harness, chatID string) []ledger.Row {
	t.Helper()
	rows, err := h.app.Repositories.ChatLedger.Read(context.Background(), chatID, 0, 0)
	require.NoError(t, err, "read chat %s's ledger", chatID)
	return rows
}

// toolRun is one tool call of the main agent as its rows tell it.
type toolRun struct {
	ID, Name, TurnID string
	Closed           bool
}

// toolRuns are the main agent's tool calls in chatID's rows, in call order.
func toolRuns(t *testing.T, h *harness, chatID string) []toolRun {
	t.Helper()
	return runsOf(chatRows(t, h, chatID))
}

func runsOf(rows []ledger.Row) []toolRun {
	var out []toolRun
	at := map[string]int{}
	for _, r := range rows {
		call, err := ledger.DecodeBody[ledger.ToolCall](r)
		if r.Kind != ledger.KindToolCall || r.Scope != "" || err != nil {
			continue
		}
		i, ok := at[call.ToolID]
		if !ok {
			i, at[call.ToolID] = len(out), len(out)
			out = append(out, toolRun{ID: call.ToolID, TurnID: r.TurnID})
		}
		out[i].Name = cmp.Or(out[i].Name, call.Name)
		out[i].Closed = out[i].Closed || call.Phase == ledger.ToolPost || call.Phase == ledger.ToolFail
	}
	return out
}
