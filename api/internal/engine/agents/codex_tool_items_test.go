package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// codex's tool items are a sum type and each variant keeps its target, its output
// and its duration in a DIFFERENT field. The descriptor mapped `tool_result:
// item.content`, a field that exists on no codex item at all, and mapped no
// tool_target and no duration_ms.
//
// The visible effect: `describeTool` renders `name · target`, so a codex turn that
// ran three commands and edited two files drew the literal word list
// "commandExecution / commandExecution / commandExecution / fileChange / fileChange"
// — no command line, no path, no output, no duration. Claude, mapping the same
// canonical fields, renders "Bash · rg --files -g '*.go'".
//
// Payload shapes are from codex's own generated schema, ThreadItem in
// codex-rs/app-server-protocol/schema/json/ServerNotification.json.
func TestRegression_CodexToolCallsCarryATargetAndAResult(t *testing.T) {
	for _, tc := range []struct {
		variant    string
		raw        string
		wantName   string
		wantTarget string
		wantResult string
		wantDurMS  int
	}{
		{
			variant: "commandExecution",
			raw: `{"item":{"type":"commandExecution","id":"c1","command":"rg --files -g '*.go'",
			       "cwd":"/w","status":"completed","aggregatedOutput":"main.go\n","exitCode":0,
			       "durationMs":412},"threadId":"t","turnId":"tn"}`,
			wantName: "commandExecution", wantTarget: "rg --files -g '*.go'",
			wantResult: "main.go\n", wantDurMS: 412,
		},
		{
			// FileUpdateChange.kind is an OBJECT ({"type":"update"}), so the changed
			// path is reachable only by index selection, not by [field=value].
			variant: "fileChange",
			raw: `{"item":{"type":"fileChange","id":"f1","status":"completed",
			       "changes":[{"path":"/w/main.go","kind":{"type":"update"},"diff":"@@"}]},
			       "threadId":"t","turnId":"tn"}`,
			wantName: "fileChange", wantTarget: "/w/main.go",
		},
		{
			// An MCP call's NAME is the tool; the server is the target.
			variant: "mcpToolCall",
			raw: `{"item":{"type":"mcpToolCall","id":"m1","server":"crowbar","tool":"get_chat_log",
			       "status":"completed","arguments":{},"result":{"content":[]},"durationMs":88},
			       "threadId":"t","turnId":"tn"}`,
			wantName: "get_chat_log", wantTarget: "crowbar", wantDurMS: 88,
		},
		{
			variant: "webSearch",
			raw: `{"item":{"type":"webSearch","id":"w1","query":"codex notifications"},
			       "threadId":"t","turnId":"tn"}`,
			wantName: "webSearch", wantTarget: "codex notifications",
		},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			ev, err := get(t, "codex").ParseHook(agents.HookToolPost, []byte(tc.raw))

			require.NoError(t, err)
			require.NotNil(t, ev.Tool)
			assert.Equal(t, tc.wantName, ev.Tool.Name)
			assert.Equal(t, tc.wantTarget, ev.Tool.Target,
				"an unmapped target renders the tool as a bare type name")
			assert.Equal(t, tc.wantDurMS, ev.Tool.DurationMS)
			if tc.wantResult != "" {
				assert.Equal(t, tc.wantResult, string(ev.Tool.Result))
			}
		})
	}
}

// tool_pre must name the same target tool_post does, or the running row in the
// working line and the finished row under the message disagree.
func TestRegression_CodexToolPreCarriesTheSameTarget(t *testing.T) {
	raw := []byte(`{"item":{"type":"commandExecution","id":"c1","command":"go build ./...",
	  "cwd":"/w","status":"inProgress"},"threadId":"t","turnId":"tn"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookToolPre, []byte(raw))

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "go build ./...", ev.Tool.Target)
	assert.Equal(t, "c1", ev.Tool.ID)
}

// A failed tool must reach tool_fail with an error, not tool_post with a silent OK.
func TestRegression_CodexFailedToolCarriesItsError(t *testing.T) {
	raw := []byte(`{"item":{"type":"mcpToolCall","id":"m1","server":"crowbar","tool":"get_chat_log",
	  "status":"failed","arguments":{},"error":{"message":"server closed the connection"},
	  "durationMs":12},"threadId":"t","turnId":"tn"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookToolFail, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "server closed the connection", ev.Tool.Error)
	assert.Equal(t, "get_chat_log", ev.Tool.Name)
}

// Reasoning is where codex spends most of a hard turn, emitting nothing else
// while it does — so with this unmapped the chat showed a spinner and a rotating
// flavour verb for all of it. Payload shape is from a LIVE capture against
// codex-cli 0.149.1 (testdata/fixtures/codex/item_reasoning_summaryTextDelta.json).
func TestRegression_CodexStreamsItsReasoning(t *testing.T) {
	raw := []byte(`{"threadId":"01a081b0-fe3e-7610-bd5d-341d4c4f7749",
	  "turnId":"01a081b0-ffd3-7a11-a381-ec122201eb91",
	  "itemId":"rs_052b6896240d8d9d016aa02d538cd087d2b6f5760993ec63b9",
	  "delta":"**Clarifying ambiguous wording**","summaryIndex":0}`)

	ev, err := get(t, "codex").ParseHook(agents.HookReasoningDelta, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, "**Clarifying ambiguous wording**", ev.Delta.Text)
	assert.Equal(t, "rs_052b6896240d8d9d016aa02d538cd087d2b6f5760993ec63b9", ev.Delta.MessageID)
	assert.Equal(t, "01a081b0-ffd3-7a11-a381-ec122201eb91", ev.Delta.TurnID)
	assert.Equal(t, 0, ev.Delta.Index, "summaryIndex orders the parts of one thinking block")
}

// A thought is not an answer. Mapping reasoning onto message_delta would have
// recorded the model's thinking as its reply, so the two must stay separate
// events and a provider that streams no reasoning must not claim to.
func TestAgent_ReasoningAndMessageDeltasAreDistinctEvents(t *testing.T) {
	assert.NotEqual(t, agents.HookMessageDelta, agents.HookReasoningDelta)

	_, err := get(t, "claude").ParseHook(agents.HookReasoningDelta, []byte(`{"delta":"x"}`))
	require.Error(t, err, "claude declares no reasoning stream, and must not claim one")
}
