//go:build integration

package tests

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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

// scopedToolIDs are the tool calls the feed records under scope, by their first row.
func (c turnChat) scopedToolIDs(t *testing.T, scope string) []string {
	t.Helper()
	var out []string
	for _, r := range ledgerRows(t, c.h, c.chatID) {
		b, err := ledger.DecodeBody[ledger.ToolCall](r)
		if r.Kind == ledger.KindToolCall && r.Scope == scope && err == nil && !slices.Contains(out, b.ToolID) {
			out = append(out, b.ToolID)
		}
	}
	return out
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

	assert.Equal(t, []string{fixtureField(t, "10-PreToolUse.json", "tool_use_id")}, c.scopedToolIDs(t, subagent),
		"the subagent's own tool call is recorded under it")
}

// backgroundActivity is each subagent's state and each tool call's status
// ("running" until a row closes it), by the fixture file that opened it.
func (c turnChat) backgroundActivity(t *testing.T, files ...string) map[string]string {
	t.Helper()
	rows := ledgerRows(t, c.h, c.chatID)
	var book ledger.SubagentBook
	got := map[string]string{}
	for _, r := range rows {
		book.Apply(r)
		if b, err := ledger.DecodeBody[ledger.ToolCall](r); r.Kind == ledger.KindToolCall && err == nil {
			if _, seen := got[b.ToolID]; !seen || b.Phase != ledger.ToolPre {
				got[b.ToolID] = cmp.Or(b.Status, "running")
			}
		}
	}
	for _, s := range book.All() {
		got[s.Scope] = "running"
		if !s.Status.Live() {
			got[s.Scope] = "stopped"
		}
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

// Both subagents run past the turn end at 11, with the tools they open after
// it, until each is over: a3edc… at its hand-back (23).
func TestRegression_ABackgroundSubagentRunsPastItsTurnsEnd(t *testing.T) {
	c := newTurnChat(t, newHarness(t))

	c.replayRange(t, 0, 13)
	assert.Equal(t, map[string]string{
		"05-SubagentStart.json": "running", "06-SubagentStart.json": "running",
		"10-PreToolUse.json": "running", "12-PreToolUse.json": "running",
	}, c.backgroundActivity(t, "05-SubagentStart.json", "06-SubagentStart.json", "10-PreToolUse.json", "12-PreToolUse.json"))

	c.replayRange(t, 13, 24)
	assert.Equal(t, map[string]string{
		"05-SubagentStart.json": "stopped", "06-SubagentStart.json": "running",
		"13-PreToolUse.json": "ok", "14-PreToolUse.json": "ok", "21-PreToolUse.json": "running",
	}, c.backgroundActivity(t, "05-SubagentStart.json", "06-SubagentStart.json",
		"13-PreToolUse.json", "14-PreToolUse.json", "21-PreToolUse.json"))
}

// 11-Stop ends the parent turn while a3edc… and a6fec… run; 23 hands a3edc…'s
// result back and opens the turn 28-Stop closes; a6fec… stops at 26.
func TestRegression_ABackgroundSubagentKeepsTheChatWorkingUntilItStops(t *testing.T) {
	c := newTurnChat(t, newHarness(t))

	c.replayRange(t, 0, 12)
	assert.True(t, c.working(t), "the parent turn ended, its subagents did not")
	c.replayRange(t, 12, 26)
	assert.True(t, c.working(t), "a6fec… still runs")
	c.replayRange(t, 26, 29)

	assert.False(t, c.working(t), "every subagent stopped and the hand-back's turn closed")
	assert.Contains(t, assistantTexts(c.messages(t)), fixtureField(t, "28-Stop.json", "last_assistant_message"),
		"the hand-back's reply is recorded")
}

// A todo list's steps arrive one by one, each a plan row; the plan is the whole list.
func TestRegression_ATodoListBecomesPlanRows(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	for id, subject := range map[string]string{"1": "Read a.txt", "2": "Run echo"} {
		c.hook(t, c.launch, "plan_step", map[string]any{
			"hook_event_name": "TaskCreated", "prompt_id": "A", "task_id": id, "task_subject": subject,
		})
	}
	c.hook(t, c.launch, "plan_step", map[string]any{
		"hook_event_name": "PostToolUse", "prompt_id": "A", "tool_name": "TaskUpdate",
		"tool_input":    map[string]any{"taskId": "1", "status": "in_progress"},
		"tool_response": map[string]any{"statusChange": map[string]any{"from": "pending", "to": "in_progress"}},
	})

	c.h.Quiesce()
	steps := map[string]string{}
	for _, s := range chatState(t, c.h, c.chatID).Plan() {
		steps[s.Text] = s.Status
	}
	assert.Equal(t, map[string]string{"Read a.txt": "active", "Run echo": "pending"}, steps)
}

// A child conversation's frame that precedes its bind is held through a late
// end of another turn, and recorded under its scope once the spawn binds.
func TestRegression_AChildsEarlyFrameSurvivesALateTurnEnd(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	c.hook(t, c.launch, "tool_pre", map[string]any{
		"prompt_id": "A", "tool_use_id": "spawn-1", "tool_name": "Agent", "tool_input": map[string]any{"description": "look"},
	})
	postSessionHook(t, c.launch, c.h.url+repoBase(c.imported), "claude", "tool_pre", map[string]any{
		"session_id": "child", "prompt_id": "A", "tool_use_id": "c1", "tool_name": "Bash",
	})
	c.hook(t, c.launch, "turn_stop", map[string]any{"prompt_id": "Z", "last_assistant_message": "late"})
	require.True(t, c.working(t), "precondition: the late end closed nothing")

	c.hook(t, c.launch, "tool_post", map[string]any{
		"prompt_id": "A", "tool_use_id": "spawn-1", "tool_name": "Agent", "tool_response": map[string]any{"agentId": "child"},
	})
	c.h.Quiesce()

	assert.Equal(t, []string{"c1"}, c.scopedToolIDs(t, "child"), "the held frame is drained into its scope")
}

func TestRegression_Symptom3_TextSaidBeforeAProcessExitsIsKept(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	c.hook(t, c.launch, "message_delta", map[string]any{
		"prompt_id": "A", "turn_id": "t-1", "message_id": "m-1", "index": 0, "final": true, "delta": "KEPT",
	})

	c.launch.Kill(t)
	c.feed.awaitRunnerGone(c.chatID, c.launch.RunnerID)
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

	_ = c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.chatID+"/stop", nil, http.StatusNoContent).Body.Close()
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
	postHook(t, h, base, "answerstub", runnerID, "user_prompt", `{"prompt":"go"}`)

	tool := `{"session_id":"s1","tool_use_id":"tool-1","tool_name":"Bash","tool_input":{"command":"touch X"}}`
	ask := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"touch X"}}`
	var held []<-chan heldHook
	for range 2 {
		postHook(t, h, base, "answerstub", runnerID, "tool_pre", tool)
		held = append(held, askHook(t.Context(), t, h, base, "answerstub", runnerID, "permission", ask))
		heldChoice(t, h, chatID)
	}
	require.NoError(t, (<-held[0]).err, "the retry is the same ask, so it takes over the first hold")
	h.Quiesce()

	assert.Len(t, toolCalls(t, h, chatID), 1, "a retried tool hook is one tool call")
	assert.Len(t, asksOf(t, h, chatID), 1, "a retried permission is one card")
}

func TestRegression_ATurnFailureReasonStaysVisible(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	c.hook(t, c.launch, "turn_failed", map[string]any{
		"prompt_id": "A", "error": "rate_limit", "last_assistant_message": "usage limit reached",
	})
	assert.False(t, c.working(t))

	failures := rowsOf(t, c.feedRows(t), ledger.KindTurnFailed, func(ledger.TurnFailed) bool { return true })
	require.Len(t, failures, 1, "the failure is one notice")
	assert.Contains(t, cmp.Or(failures[0].Detail, failures[0].Reason), "usage limit")
}

func TestRegression_ATurnFailureCarriesTheKindItsDescriptorClassified(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	c.hook(t, c.launch, "turn_failed", map[string]any{"prompt_id": "A", "error": "rate_limit"})

	failures := rowsOf(t, c.feedRows(t), ledger.KindTurnFailed, func(ledger.TurnFailed) bool { return true })
	require.Len(t, failures, 1)
	assert.Equal(t, "limit", failures[0].Kind)
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
