//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// newFakeClaudeChat installs the shipped claude descriptor over the fake CLI,
// creates a chat on it, and returns the chat, its launch capture and the hook
// route prefix a real claude would post to.
func newFakeClaudeChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (chatID string, launch kit.Launch, hookBase string) {
	t.Helper()
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": fake.ID, "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()

	detail := getAgentChat(t, h, repoBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the fake CLI must be placed on the new chat")
	return created.ID, fake.AwaitLaunch(t, detail.LiveRunnerID), h.url + repoBase(imported)
}

func TestFakeProvider_LaunchIsCapturedAndHooksReachTheChat(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	chatID, l, hookBase := newFakeClaudeChat(t, h, imported)

	require.NotEmpty(t, l.Token, "the runner token is what authenticates every hook")
	assert.NotContains(t, l.Argv, "--resume", "a fresh chat launches without a resume")

	for _, step := range []struct{ event, payload string }{
		{"session_start", `{"session_id":"s-1","source":"startup"}`},
		{"user_prompt", `{"session_id":"s-1","prompt":"hello"}`},
		// Claude sends MessageDisplay before Stop; a Stop with text and no open message waits 3s.
		{"message_delta", `{"session_id":"s-1","turn_id":"t-1","message_id":"m-1","index":0,"final":true,"delta":"HI"}`},
		{"turn_stop", `{"session_id":"s-1","last_assistant_message":"HI"}`},
	} {
		resp := l.PostHook(t, hookBase, "claude", step.event, []byte(step.payload)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
		require.Equal(t, http.StatusOK, resp.StatusCode, step.event)
	}
	h.Quiesce()

	messages := readRecordedMessages(t, h, imported, chatID)
	roles, texts := []string{}, []string{}
	for _, m := range messages {
		roles = append(roles, m.Role)
		texts = append(texts, m.Text)
	}
	assert.Equal(t, []string{"user", "assistant"}, roles)
	assert.Equal(t, []string{"hello", "HI"}, texts)
}

func TestFakeProvider_ReplaysTheRecordedClaudeSubagentSession(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	chatID, l, hookBase := newFakeClaudeChat(t, h, imported)

	dir := filepath.Join("testdata", "hooks", "claude-subagent-session")
	responses, skipped := l.ReplayHooks(t, hookBase, "claude", dir, kit.ClaudeHookRoutes, nil) //nolint:bodyclose // PostHook closes each body and returns a buffered copy
	h.Quiesce()

	require.Empty(t, skipped, "every recorded hook has a route in the shipped descriptor")
	require.NotEmpty(t, responses)
	for _, resp := range responses {
		assert.Less(t, resp.StatusCode, http.StatusInternalServerError, resp.Request.URL.Path)
	}

	var first struct {
		Prompt string `json:"prompt"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, "01-UserPromptSubmit.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &first))
	var userTexts []string
	for _, m := range readRecordedMessages(t, h, imported, chatID) {
		if m.Role == "user" {
			userTexts = append(userTexts, m.Text)
		}
	}
	assert.Contains(t, userTexts, first.Prompt)

	var activity struct {
		ToolCalls []json.RawMessage `json:"toolCalls"`
	}
	h.get(repoBase(imported)+"/chats/"+chatID+"/activity", &activity)
	assert.NotEmpty(t, activity.ToolCalls, "the recorded session ran tools")
}

func TestFakeProvider_KillEndsTheRunnerWithNoHook(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	chatID, l, _ := newFakeClaudeChat(t, h, imported)

	conn := h.dial(repoBase(imported) + "/chats/ws")
	l.Kill(t)
	readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "exited" && m["runnerId"] == l.RunnerID
	})
	h.QuiesceReactors()

	assert.Empty(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID,
		"a CLI that exits with no hook leaves the chat dormant")
}
