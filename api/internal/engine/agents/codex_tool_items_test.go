package agents_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestRegression_CodexToolCallsCarryATargetAndAResult(t *testing.T) {
	for _, tc := range []struct {
		variant    string
		raw        string
		wantName   string
		wantTarget string
		wantResult string
		wantDurMS  int
		wantServer string
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
			variant: "fileChange",
			raw: `{"item":{"type":"fileChange","id":"f1","status":"completed",
			       "changes":[{"path":"/w/main.go","kind":{"type":"update"},"diff":"@@"}]},
			       "threadId":"t","turnId":"tn"}`,
			wantName: "fileChange", wantTarget: "/w/main.go",
		},
		{
			variant: "mcpToolCall",
			raw: `{"item":{"type":"mcpToolCall","id":"m1","server":"crowbar","tool":"get_chat_log",
			       "status":"completed","arguments":{},"result":{"content":[]},"durationMs":88},
			       "threadId":"t","turnId":"tn"}`,
			wantName: "get_chat_log", wantServer: "crowbar", wantDurMS: 88,
		},
		{
			variant: "webSearch",
			raw: `{"item":{"type":"webSearch","id":"w1","query":"codex notifications"},
			       "threadId":"t","turnId":"tn"}`,
			wantName: "webSearch", wantTarget: "codex notifications",
		},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			ev, err := get(t, "codex").ParseHook(agents.HookToolPost, []byte(tc.raw), agents.ChannelAPI)

			require.NoError(t, err)
			require.NotNil(t, ev.Tool)
			assert.NotEmpty(t, ev.Tool.Kind)
			assert.Equal(t, tc.wantName, ev.Tool.Name)
			assert.Equal(t, tc.wantTarget, ev.Tool.Target,
				"an unmapped target renders the tool as a bare type name")
			assert.Equal(t, tc.wantServer, ev.Tool.MCPServer)
			assert.Equal(t, tc.wantDurMS, ev.Tool.DurationMS)
			if tc.wantResult != "" {
				assert.Equal(t, tc.wantResult, string(ev.Tool.Result))
			}
			if tc.variant == "fileChange" {
				assert.Equal(t, []agents.ToolLocation{{Path: "/w/main.go"}}, ev.Tool.Locations)
			}
		})
	}
}

func TestCodexFileChangeKeepsItsOwnFilesDiffOnTheTool(t *testing.T) {
	raw := []byte(`{"item":{"type":"fileChange","id":"edit-1","status":"completed",
		"changes":[{"path":"/w/a.go","diff":"@@ -1 +1 @@\n-old-a\n+new-a\n"},
		{"path":"/w/b.go","diff":"@@ -1 +1 @@\n-old-b\n+new-b\n"}]},
		"threadId":"thread-1","turnId":"turn-1"}`)
	for _, hook := range []string{agents.HookToolPre, agents.HookToolPost} {
		ev, err := get(t, "codex").ParseHook(hook, raw, agents.ChannelAPI)
		require.NoError(t, err)
		require.NotNil(t, ev.Tool)
		assert.Equal(t, "--- a/w/a.go\n+++ b/w/a.go\n@@ -1 +1 @@\n-old-a\n+new-a\n\n"+
			"--- a/w/b.go\n+++ b/w/b.go\n@@ -1 +1 @@\n-old-b\n+new-b\n", ev.Tool.Diff)
	}
}

func TestRegression_CodexAddedAndDeletedFilesDiffAsWholeFiles(t *testing.T) {
	for _, tc := range []struct {
		hook, file, want string
	}{
		{agents.HookToolPost, "item_completed.fileChange.json", "--- /dev/null\n+++ b/outside-cx.txt\n@@ -0,0 +1 @@\n+hi\n\n" +
			"--- a/ws/notes.txt\n+++ b/ws/notes.txt\n@@ -1,3 +1,3 @@\n alpha\n-beta\n+BETA\n gamma\n\n" +
			"--- /dev/null\n+++ b/ws/src/util.go\n@@ -0,0 +1,5 @@\n+package src\n+\n+func Add(a, b int) int {\n+\treturn a + b\n+}\n"},
		{agents.HookToolPre, "item_started.fileChange.json", "--- /dev/null\n+++ b/outside-cx.txt\n@@ -0,0 +1 @@\n+hi\n\n" +
			"--- a/ws/notes.txt\n+++ b/ws/notes.txt\n@@ -1,3 +1,3 @@\n alpha\n-beta\n+BETA\n gamma\n\n" +
			"--- /dev/null\n+++ b/ws/src/util.go\n@@ -0,0 +1,5 @@\n+package src\n+\n+func Add(a, b int) int {\n+\treturn a + b\n+}\n"},
		{
			agents.HookToolPost, "item_completed.fileChange.delete.json",
			"--- a/ws/src/util.go\n+++ /dev/null\n@@ -1,5 +0,0 @@\n-package src\n-\n-func Add(a, b int) int {\n-\treturn a + b\n-}\n",
		},
		{agents.HookToolFail, "item_completed.fileChangeDeclined.json", "--- /dev/null\n+++ b/outside2-cx.txt\n@@ -0,0 +1 @@\n+hi\n"},
	} {
		ev, err := get(t, "codex").ParseHook(tc.hook, codexParams(t, tc.file), agents.ChannelAPI)

		require.NoError(t, err, tc.file)
		require.NotNil(t, ev.Tool, tc.file)
		assert.Equal(t, tc.want, ev.Tool.Diff, tc.file)
	}
}

func TestRegression_CodexDenyCarriesNoReason(t *testing.T) {
	for _, file := range []string{"item_fileChange_requestApproval.json", "item_commandExecution_requestApproval.json"} {
		var params map[string]any
		require.NoError(t, json.Unmarshal(codexParams(t, file), &params))
		params["tool"] = "fileChange"
		raw, err := json.Marshal(params)
		require.NoError(t, err)

		ev, err := get(t, "codex").ParseHook(agents.HookPermission, raw, agents.ChannelAPI)

		require.NoError(t, err, file)
		require.NotNil(t, ev.Choice, file)
		assert.False(t, denyOption(t, ev.Choice.Options).Reason, file)
	}
}

func TestRegression_CodexOffersTheBroaderDecisionsItLists(t *testing.T) {
	var params map[string]any
	require.NoError(t, json.Unmarshal(codexParams(t, "item_commandExecution_requestApproval.json"), &params))
	params["tool"] = "commandExecution"
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	codex := get(t, "codex")

	ev, err := codex.ParseHook(agents.HookPermission, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	var offered []agents.ChoiceOption
	for _, o := range ev.Choice.Options {
		if o.Kind == agents.ChoiceOptionSuggestion {
			offered = append(offered, o)
		}
	}
	require.Len(t, offered, 1, "%+v", ev.Choice.Options)
	assert.Equal(t, "Always allow commands that start like this", offered[0].Label)
	reply, err := codex.RenderAnswer(agents.HookPermission, raw,
		agents.AnswerDecision{Key: agents.ChoiceOptionSuggestion, Picked: []string{offered[0].ID}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"decision":{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["curl"]}}}`, string(reply))
}

func denyOption(t *testing.T, options []agents.ChoiceOption) agents.ChoiceOption {
	t.Helper()
	for _, o := range options {
		if o.Kind == agents.ChoiceOptionDeny {
			return o
		}
	}
	require.Fail(t, "no deny option", "%+v", options)
	return agents.ChoiceOption{}
}

func TestRegression_CodexReportsTheLatestTurnDiff(t *testing.T) {
	raw := []byte(`{"threadId":"thread-1","turnId":"turn-1","diff":"diff --git a/a b/a\n"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookDiffUpdate, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, "turn-1", ev.Delta.MessageID)
	assert.Equal(t, "diff --git a/a b/a\n", ev.Delta.Text)
}

func TestRegression_CodexPreservesInterruptedTurnStatus(t *testing.T) {
	raw := []byte(`{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}`)

	ev, err := get(t, "codex").ParseHook(agents.HookTurnStop, raw, agents.ChannelAPI)

	require.NoError(t, err)
	assert.Equal(t, "turn-1", ev.TurnID)
	assert.Equal(t, "interrupted", ev.TurnStatus)
}

func TestRegression_CodexToolPreCarriesTheSameTarget(t *testing.T) {
	raw := []byte(`{"item":{"type":"commandExecution","id":"c1","command":"go build ./...",
	  "cwd":"/w","status":"inProgress"},"threadId":"t","turnId":"tn"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookToolPre, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "go build ./...", ev.Tool.Target)
	assert.Equal(t, "c1", ev.Tool.ID)
}

func TestRegression_CodexFailedToolCarriesItsError(t *testing.T) {
	raw := []byte(`{"item":{"type":"mcpToolCall","id":"m1","server":"crowbar","tool":"get_chat_log",
	  "status":"failed","arguments":{},"error":{"message":"server closed the connection"},
	  "durationMs":12},"threadId":"t","turnId":"tn"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookToolFail, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "server closed the connection", ev.Tool.Error)
	assert.Equal(t, "get_chat_log", ev.Tool.Name)
	assert.Equal(t, "error", ev.Tool.Status)
}

func TestRegression_CodexDeclinedToolKeepsItsDistinctStatus(t *testing.T) {
	raw := []byte(`{"item":{"type":"commandExecution","id":"c1","command":"rm file",
	  "status":"declined","aggregatedOutput":"","durationMs":1},"threadId":"t","turnId":"tn"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookToolFail, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "declined", ev.Tool.Status)
}

func TestRegression_CodexStreamsItsReasoning(t *testing.T) {
	raw := []byte(`{"threadId":"01a081b0-fe3e-7610-bd5d-341d4c4f7749",
	  "turnId":"01a081b0-ffd3-7a11-a381-ec122201eb91",
	  "itemId":"rs_052b6896240d8d9d016aa02d538cd087d2b6f5760993ec63b9",
	  "delta":"**Clarifying ambiguous wording**","summaryIndex":0}`)

	ev, err := get(t, "codex").ParseHook(agents.HookReasoningDelta, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, "**Clarifying ambiguous wording**", ev.Delta.Text)
	assert.Equal(t, "rs_052b6896240d8d9d016aa02d538cd087d2b6f5760993ec63b9", ev.Delta.MessageID)
	assert.Equal(t, "01a081b0-ffd3-7a11-a381-ec122201eb91", ev.Delta.TurnID)
	assert.Equal(t, 0, ev.Delta.Index, "summaryIndex orders the parts of one thinking block")
}

func TestAgent_ReasoningAndMessageDeltasAreDistinctEvents(t *testing.T) {
	assert.NotEqual(t, agents.HookMessageDelta, agents.HookReasoningDelta)

	_, err := get(t, "claude").ParseHook(agents.HookReasoningDelta, []byte(`{"delta":"x"}`), agents.ChannelHooks)
	require.Error(t, err, "claude declares no reasoning stream, and must not claim one")
}

func TestRegression_CodexStreamsARunningCommandsOutput(t *testing.T) {
	raw := []byte(`{"threadId":"01a081af-73bf-7fd1-bc4b-efd041f46edc",
	  "turnId":"01a081af-7550-79d0-a51e-6424d9724c88",
	  "itemId":"call_PQbUMzNNrlEGDg3Y4zc7JwNW","delta":"line 2\n"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookToolOutputDelta, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, "line 2\n", ev.Delta.Text)
	assert.Equal(t, "call_PQbUMzNNrlEGDg3Y4zc7JwNW", ev.Delta.MessageID)
}

func TestRegression_CodexReportsItsPlan(t *testing.T) {
	raw := []byte(`{"threadId":"01a081af-73bf-7fd1-bc4b-efd041f46edc",
	  "turnId":"01a081af-7550-79d0-a51e-6424d9724c88","explanation":null,
	  "plan":[{"step":"Run the requested five-line shell command exactly.","status":"inProgress"},
	          {"step":"List the top-level directory contents.","status":"pending"},
	          {"step":"Check repository status, then summarize.","status":"completed"}]}`)

	ev, err := get(t, "codex").ParseHook(agents.HookPlanUpdate, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.Len(t, ev.Plan, 3)
	assert.Equal(t, "Run the requested five-line shell command exactly.", ev.Plan[0].Text)
	assert.Equal(t, agents.PlanStepActive, ev.Plan[0].Status)
	assert.Equal(t, agents.PlanStepPending, ev.Plan[1].Status)
	assert.Equal(t, agents.PlanStepDone, ev.Plan[2].Status)
}

func TestAgent_AnUnmappedPlanStatusPassesThrough(t *testing.T) {
	raw := []byte(`{"threadId":"t","turnId":"tn",
	  "plan":[{"step":"something","status":"somethingCodexAddedLater"}]}`)

	ev, err := get(t, "codex").ParseHook(agents.HookPlanUpdate, raw, agents.ChannelAPI)

	require.NoError(t, err)
	require.Len(t, ev.Plan, 1)
	assert.Equal(t, "somethingCodexAddedLater", ev.Plan[0].Status)
}

func TestRegression_CodexUserPromptNamesItsTurn(t *testing.T) {
	raw := []byte(`{"item":{"type":"userMessage","id":"u1","content":[{"type":"text","text":"hi"}]},
	  "threadId":"thread-1","turnId":"turn-1"}`)

	ev, err := get(t, "codex").ParseHook(agents.HookUserPrompt, raw, agents.ChannelAPI)

	require.NoError(t, err)
	assert.Equal(t, "turn-1", ev.TurnID)
}

func TestCodexWarningsReachTheChatAsWarnings(t *testing.T) {
	for file, want := range map[string]string{
		"warning.json":       "recorded with model",
		"configWarning.json": "bubblewrap",
	} {
		ev, err := get(t, "codex").ParseHook(agents.HookWarning, codexParams(t, file), agents.ChannelAPI)

		require.NoError(t, err, file)
		assert.Contains(t, ev.Message, want, file)
	}
}

func codexParams(t *testing.T, file string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("internal", "protocol", "testdata", "fixtures", "codex", file))
	require.NoError(t, err)
	var frame struct {
		Params json.RawMessage `json:"params"`
	}
	require.NoError(t, json.Unmarshal(raw, &frame))
	return frame.Params
}
