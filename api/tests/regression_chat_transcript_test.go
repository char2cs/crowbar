//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// recordedSubagentSession is the session id the subagent fixtures were recorded on.
const (
	subagentSessionDir      = "testdata/hooks/claude-subagent-session"
	recordedSubagentSession = "68a2773c-78c5-4edd-b4fe-bf5bd1309895"
)

// replayRange replays the recorded hooks numbered [from, to) on c's session.
func (c turnChat) replayRange(t *testing.T, from, to int) {
	t.Helper()
	entries, err := os.ReadDir(subagentSessionDir)
	require.NoError(t, err)
	dir := t.TempDir()
	for _, e := range entries {
		n, err := strconv.Atoi(strings.SplitN(e.Name(), "-", 2)[0])
		if err != nil || n < from || n >= to {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(subagentSessionDir, e.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o600))
	}
	session := argAfter(c.launch.Argv, "--session-id")
	responses, skipped := c.launch.ReplayHooks(t, c.h.url+repoBase(c.imported), "claude", dir, //nolint:bodyclose // PostHook closes each body and returns a buffered copy
		kit.ClaudeHookRoutes, map[string]string{recordedSubagentSession: session})
	require.Empty(t, skipped)
	for _, resp := range responses {
		require.Less(t, resp.StatusCode, http.StatusInternalServerError, resp.Request.URL.Path)
	}
	c.h.Quiesce()
}

func fixtureField(t *testing.T, name, field string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(subagentSessionDir, name))
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	value, _ := payload[field].(string)
	require.NotEmpty(t, value, "%s carries %s", name, field)
	return value
}

func (c turnChat) messages(t *testing.T) []recordedMessage {
	t.Helper()
	return readRecordedMessages(t, c.h, c.imported, c.chatID)
}

func TestRegression_Symptom3_ATailChunkAfterStopNeverOverwritesTheReply(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	reply := fixtureField(t, "28-Stop.json", "last_assistant_message")
	tail := fixtureField(t, "29-MessageDisplay.json", "delta")

	c.replayRange(t, 0, 30)
	assert.Contains(t, assistantTexts(c.messages(t)), reply, "the turn's reply is recorded in full")
	assert.NotContains(t, assistantTexts(c.messages(t)), tail, "a tail chunk is never a reply of its own")

	c.replayRange(t, 30, 35)
	assert.Contains(t, assistantTexts(c.messages(t)), reply, "the next turn leaves the reply whole")
	assert.NotContains(t, assistantTexts(c.messages(t)), tail)
}

func TestRegression_Symptom3_ASubagentsToolCallsAreRecorded(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	subagent := fixtureField(t, "05-SubagentStart.json", "agent_id")

	c.replayRange(t, 0, 11)

	var activity struct {
		ToolCalls []struct {
			ID         string `json:"id"`
			SubagentID string `json:"subagentId"`
		} `json:"toolCalls"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/activity", &activity)
	var scoped []string
	for _, call := range activity.ToolCalls {
		if call.SubagentID == subagent {
			scoped = append(scoped, call.ID)
		}
	}
	assert.Equal(t, []string{fixtureField(t, "10-PreToolUse.json", "tool_use_id")}, scoped,
		"the subagent's own tool call is recorded under it")
}

// backgroundActivity is each subagent's state and each tool call's status,
// by the fixture file that opened it.
func (c turnChat) backgroundActivity(t *testing.T, files ...string) map[string]string {
	t.Helper()
	var activity struct {
		ToolCalls []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"toolCalls"`
		Subagents []struct {
			ID      string  `json:"id"`
			EndedAt *string `json:"endedAt"`
		} `json:"subagents"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/activity", &activity)
	got := map[string]string{}
	for _, s := range activity.Subagents {
		got[s.ID] = "running"
		if s.EndedAt != nil {
			got[s.ID] = "stopped"
		}
	}
	for _, call := range activity.ToolCalls {
		got[call.ID] = call.Status
	}
	out := map[string]string{}
	for _, f := range files {
		field := "tool_use_id"
		if strings.Contains(f, "Subagent") {
			field = "agent_id"
		}
		out[f] = got[fixtureField(t, f, field)]
	}
	return out
}

// The turn end at 11 restates both subagents as background work: they run past
// it, with the tools they open after it, until each stops.
func TestRegression_ABackgroundSubagentRunsPastItsTurnsEnd(t *testing.T) {
	c := newTurnChat(t, newHarness(t))

	c.replayRange(t, 0, 13)
	assert.Equal(t, map[string]string{
		"05-SubagentStart.json": "running", "06-SubagentStart.json": "running",
		"10-PreToolUse.json": "running", "12-PreToolUse.json": "running",
	}, c.backgroundActivity(t, "05-SubagentStart.json", "06-SubagentStart.json", "10-PreToolUse.json", "12-PreToolUse.json"))

	c.replayRange(t, 13, 24)
	assert.Equal(t, map[string]string{
		"05-SubagentStart.json": "running", "06-SubagentStart.json": "running",
		"13-PreToolUse.json": "ok", "14-PreToolUse.json": "ok", "21-PreToolUse.json": "running",
	}, c.backgroundActivity(t, "05-SubagentStart.json", "06-SubagentStart.json",
		"13-PreToolUse.json", "14-PreToolUse.json", "21-PreToolUse.json"))
}

func TestRegression_Symptom3_TextSaidBeforeAProcessExitsIsKept(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	c.hook(t, c.launch, "message_delta", map[string]any{
		"prompt_id": "A", "turn_id": "t-1", "message_id": "m-1", "index": 0, "final": true, "delta": "KEPT",
	})

	c.launch.Kill(t)
	readUntil(t, c.conn, func(m map[string]any) bool {
		return m["kind"] == "exited" && m["runnerId"] == c.launch.RunnerID
	})
	c.h.QuiesceReactors()

	assert.Equal(t, []string{"KEPT"}, assistantTexts(c.messages(t)))
}

func TestRegression_Symptom3_AStoppedTurnsLateFinalTextIsRecordedNotReopened(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	late, err := json.Marshal(map[string]any{
		"session_id": argAfter(c.launch.Argv, "--session-id"), "prompt_id": "A", "last_assistant_message": "LATE",
	})
	require.NoError(t, err)
	c.launch.OnQuit(t, c.fake, c.h.url+repoBase(c.imported), "claude", "turn_stop", late)

	_ = c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.chatID+"/stop", nil, http.StatusAccepted).Body.Close()
	c.h.QuiesceReactors()

	assert.Equal(t, []string{"LATE"}, assistantTexts(c.messages(t)), "what the CLI said on its way down is kept")
	assert.False(t, c.working(t), "a late stop reopens nothing")
}

func TestRegression_ARetriedHookDeliveryRecordsItsFactOnce(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "answerstub", retryStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "answerstub")
	base := repoBase(imported)
	postAnswerStubHook(t, h, base, runnerID, "user_prompt", `{"prompt":"go"}`)

	tool := `{"session_id":"s1","tool_use_id":"tool-1","tool_name":"Bash","tool_input":{"command":"touch X"}}`
	ask := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"touch X"}}`
	for range 2 {
		postAnswerStubHook(t, h, base, runnerID, "tool_pre", tool)
		postAnswerStubHook(t, h, base, runnerID, "permission", ask)
	}
	h.Quiesce()

	var activity struct {
		ToolCalls []json.RawMessage `json:"toolCalls"`
	}
	h.get(base+"/chats/"+chatID+"/activity", &activity)
	assert.Len(t, activity.ToolCalls, 1, "a retried tool hook is one tool call")
	var choices []json.RawMessage
	h.get(base+"/chats/"+chatID+"/choices", &choices)
	assert.Len(t, choices, 1, "a retried permission is one card")
}

func TestRegression_ATurnFailureReasonStaysVisible(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	c.hook(t, c.launch, "turn_failed", map[string]any{
		"prompt_id": "A", "error": "rate_limit", "last_assistant_message": "usage limit reached",
	})
	assert.False(t, c.working(t))

	var notices []string
	for _, m := range c.messages(t) {
		if m.Role == "notice" {
			notices = append(notices, m.Text)
		}
	}
	require.Len(t, notices, 1, "the failure is one notice")
	assert.Contains(t, notices[0], "usage limit")
}

// A turn_stop that beat its own final delta: whichever lands first, the reply
// is one message.
func TestRegression_Symptom3_AReplyIsOneMessageWhicheverOfItsHooksLandsFirst(t *testing.T) {
	for _, stopFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("stop first %v", stopFirst), func(t *testing.T) {
			c := newTurnChat(t, newHarness(t))
			c.startTurn(t, c.launch, "A")
			stop := func() {
				c.hook(t, c.launch, "turn_stop", map[string]any{"prompt_id": "A", "last_assistant_message": "ONE REPLY"})
			}
			final := func() {
				c.hook(t, c.launch, "message_delta", map[string]any{
					"prompt_id": "A", "turn_id": uuid.NewString(), "message_id": "m-1",
					"index": 0, "final": true, "delta": "ONE REPLY",
				})
			}
			if stopFirst {
				stop()
				final()
			} else {
				final()
				stop()
			}
			c.h.Quiesce()
			assert.Equal(t, []string{"ONE REPLY"}, assistantTexts(c.messages(t)))
		})
	}
}

const retryStubProviderDescriptorYAML = `id: answerstub
spawn:
  cmd: "cat"
  interactive_required: true
events:
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  tool_pre:
    in: tool_pre
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
  permission:
    ask: permission
    timeout_seconds: 5
    map:
      session_id: session_id
      message:    tool_name
      tool_name:  tool_name
      tool_target: tool_input.command
      tool_input: tool_input
    reply:
      allow: '{"decision":{"behavior":"allow"}}'
      deny: '{"decision":{"behavior":"deny","message":{reason_json}}}'
runtime:
  transport: hooks
  hooks:
    format: json
`
