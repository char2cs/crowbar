//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

var stubs sync.Map

var stubLifecycle = map[string]map[string]any{
	"session_start": {"in": "session_start", "map": map[string]any{"session_id": "session_id"}},
	"user_prompt":   {"in": "user_prompt", "map": map[string]any{"message": "prompt"}},
	"turn_stop":     {"in": "turn_stop", "map": map[string]any{"session_id": "session_id", "message": "last_assistant_message"}},
}

func writeProviderDescriptor(t *testing.T, h *harness, id, body string) {
	t.Helper()
	var stub map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(body), &stub))
	events, _ := stub["events"].(map[string]any)
	if events == nil {
		events = map[string]any{}
	}
	for name, spec := range stubLifecycle {
		if _, ok := events[name]; !ok {
			events[name] = spec
		}
	}
	raw, err := yaml.Marshal(events)
	require.NoError(t, err)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{ID: id, EventsYAML: string(raw), Patch: func(doc map[string]any) {
		for key := range doc {
			if key != "spawn" && key != "events" && key != "hooks_injection" {
				delete(doc, key)
			}
		}
		for key, v := range stub {
			if key != "id" && key != "events" {
				doc[key] = v
			}
		}
	}})
	key := h.home + "|" + id
	stubs.Store(key, fake)
	t.Cleanup(func() { stubs.Delete(key) })
}

func stubLaunch(t *testing.T, h *harness, id, runnerID string) kit.Launch {
	t.Helper()
	stored, _ := stubs.Load(h.home + "|" + id)
	fake, ok := stored.(kit.FakeProvider)
	require.True(t, ok, "stub provider %q was never installed", id)
	return fake.AwaitLaunch(t, runnerID)
}

func postHook(t *testing.T, h *harness, base, provider, runnerID, event, payload string) []byte {
	t.Helper()
	resp := stubLaunch(t, h, provider, runnerID).PostHook(t, h.url+base, provider, event, []byte(payload)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", event, body)
	return body
}

type heldHook struct {
	status int
	body   string
	err    error
}

func askHook(ctx context.Context, t *testing.T, h *harness, base, provider, runnerID, event, payload string) <-chan heldHook {
	t.Helper()
	l := stubLaunch(t, h, provider, runnerID)
	url := fmt.Sprintf("%s%s/chats/runners/%s/hooks/%s/%s", h.url, base, runnerID, provider, event)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	req.Header.Set("X-Crowbar-Runner-Token", l.Token)
	req.Header.Set("Content-Type", "application/json")
	out := make(chan heldHook, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			out <- heldHook{err: err}
			return
		}
		body, err := io.ReadAll(resp.Body)
		out <- heldHook{status: resp.StatusCode, body: string(body), err: errors.Join(err, resp.Body.Close())}
	}()
	return out
}

func heldChoice(t *testing.T, h *harness, chatID string, seen ...string) string {
	t.Helper()
	var id string
	require.Eventually(t, func() bool {
		for _, c := range openAsks(asksOf(t, h, chatID)) {
			if c.Answerable && !slices.Contains(seen, c.ID) {
				id = c.ID
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond, "no held ask opened on chat %s", chatID)
	return id
}

func newFakeClaudeChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (chatID string, launch kit.Launch, hookBase string) {
	t.Helper()
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, launch = createFakeChat(t, h, imported, fake)
	return chatID, launch, h.url + repoBase(imported)
}

func createFakeChat(t *testing.T, h *harness, imported importedRepo, fake kit.FakeProvider) (string, kit.Launch) {
	t.Helper()
	chatID := createChat(t, h, imported, fake.ID)
	return chatID, fake.AwaitLaunch(t, startChat(t, h, repoBase(imported), chatID))
}

func createChat(t *testing.T, h *harness, imported importedRepo, provider string) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": provider, "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()
	return created.ID
}

func startChat(t *testing.T, h *harness, base, chatID string) string {
	t.Helper()
	var started struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats/"+chatID+"/resume", nil, http.StatusOK, &started)
	require.NotEmpty(t, started.ID, "a start places a runner on the chat")
	h.QuiesceReactors()
	return started.ID
}

func TestFakeProvider_LaunchIsCapturedAndHooksReachTheChat(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	chatID, l, hookBase := newFakeClaudeChat(t, h, imported)

	require.NotEmpty(t, l.Token, "the runner token is what authenticates every hook")
	assert.NotContains(t, l.Argv, "--resume", "a fresh chat launches without a resume")
	runHookTurn(t, h, l, hookBase, "claude")
	assertTurnRecorded(t, h, imported, chatID)
}

func TestFakeProvider_ACustomDescriptorInTheHomeRunsATurn(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude", ID: "acme", Patch: func(doc map[string]any) {
		doc["display_name"] = "Acme"
	}})

	var listed []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Enabled     bool   `json:"enabled"`
	}
	h.get("/v0/settings/chat/providers", &listed)
	names := map[string]string{}
	for _, p := range listed {
		if p.Enabled {
			names[p.ID] = p.DisplayName
		}
	}
	assert.Equal(t, "Acme", names["acme"], "Settings > Agents lists it, enabled")
	var reports []struct {
		ID       string `json:"id"`
		Findings []struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	h.get("/v0/settings/chat/descriptors", &reports)
	for _, r := range reports {
		if r.ID != "acme" {
			continue
		}
		for _, f := range r.Findings {
			assert.NotEqual(t, "error", f.Severity, f.Message)
		}
	}

	chatID, l := createFakeChat(t, h, imported, fake)
	runHookTurn(t, h, l, h.url+repoBase(imported), "acme")
	assertTurnRecorded(t, h, imported, chatID)
}

func runHookTurn(t *testing.T, h *harness, l kit.Launch, hookBase, provider string) {
	t.Helper()
	for _, step := range []struct{ event, payload string }{
		{"session_start", `{"session_id":"s-1","source":"startup"}`},
		{"user_prompt", `{"session_id":"s-1","prompt":"hello"}`},
		{"message_delta", `{"session_id":"s-1","turn_id":"t-1","message_id":"m-1","index":0,"final":true,"delta":"HI"}`},
		{"turn_stop", `{"session_id":"s-1","last_assistant_message":"HI"}`},
	} {
		resp := l.PostHook(t, hookBase, provider, step.event, []byte(step.payload)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
		require.Equal(t, http.StatusOK, resp.StatusCode, step.event)
	}
	h.Quiesce()
}

func assertTurnRecorded(t *testing.T, h *harness, imported importedRepo, chatID string) {
	t.Helper()
	roles, texts := []string{}, []string{}
	for _, m := range readRecordedMessages(t, h, imported, chatID) {
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

	assert.NotEmpty(t, toolCalls(t, h, chatID), "the recorded session ran tools")
}

func TestFakeProvider_KillEndsTheRunnerWithNoHook(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	chatID, l, _ := newFakeClaudeChat(t, h, imported)

	killAndAwaitExit(t, h, recordSidebar(t, h), chatID, l)

	assert.Empty(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID,
		"a CLI that exits with no hook leaves the chat dormant")
}

type newChatView struct {
	LiveRunnerID      string `json:"liveRunnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
	Phase             string `json:"phase"`
	Surface           string `json:"surface"`
}

func readNewChat(t *testing.T, h *harness, imported importedRepo, chatID string) newChatView {
	t.Helper()
	var view newChatView
	h.get(repoBase(imported)+"/chats/"+chatID, &view)
	return view
}

func requireOneLaunch(t *testing.T, fake kit.FakeProvider, l kit.Launch) {
	t.Helper()
	assert.Equal(t, []string{l.RunnerID}, fake.LaunchedRunners(t), "exactly one CLI was launched")
}

func TestFakeProvider_CreateThenFirstPromptLaunchesOnce(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID := createChat(t, h, imported, fake.ID)

	view := readNewChat(t, h, imported, chatID)
	assert.Empty(t, view.LiveRunnerID)
	assert.Equal(t, "dormant", view.Phase)
	assert.Empty(t, fake.LaunchedRunners(t), "the create launches nothing")

	l := promptLaunch(t, h, imported, fake, chatID, "first words", "")
	assert.Contains(t, l.Argv, "first words")
	requireOneLaunch(t, fake, l)
	assert.Empty(t, exitReason(t, h, imported, chatID), "no runner was displaced")
}

func TestFakeProvider_CreateThenReloadThenPromptLaunchesOnce(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID := createChat(t, h, imported, fake.ID)

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Empty(t, list[0].LiveRunnerID)
	assert.Equal(t, fake.ID, list[0].ActiveProviderID, "a dormant new chat still names its provider")
	assert.Empty(t, readNewChat(t, h, imported, chatID).LiveRunnerID)

	l := promptLaunch(t, h, imported, fake, chatID, "after a reload", "")
	requireOneLaunch(t, fake, l)
}

func TestFakeProvider_TerminalFirstLaunchesOnTheTerminal(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})

	chatID := createChat(t, h, imported, fake.ID)
	var opened struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch-to-terminal", nil, http.StatusOK, &opened)
	h.QuiesceReactors()
	view := readNewChat(t, h, imported, chatID)
	require.NotEmpty(t, view.LiveRunnerID, "opening the terminal launched the CLI")
	assert.Equal(t, opened.ID, view.TerminalSessionID, "the open answers the PTY it launched")
	assert.Equal(t, "terminal", view.Surface)
	requireOneLaunch(t, fake, fake.AwaitLaunch(t, view.LiveRunnerID))

	other := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude", ID: "termfake"})
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats", map[string]string{
		"provider": other.ID, "workspaceId": imported.workspaceID, "surface": "terminal",
	}, http.StatusCreated, &created)
	h.QuiesceReactors()
	born := readNewChat(t, h, imported, created.ID)
	require.NotEmpty(t, born.LiveRunnerID, "a terminal create launches with the create")
	requireOneLaunch(t, other, other.AwaitLaunch(t, born.LiveRunnerID))
}

func TestFakeProvider_AnAppServerChatLaunchesOnItsFirstPrompt(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "codex"})
	chatID := createChat(t, h, imported, fake.ID)
	assert.Empty(t, fake.LaunchedRunners(t), "the create launches nothing")

	l := promptLaunch(t, h, imported, fake, chatID, "Reply with exactly: OK", "")
	require.Eventually(t, func() bool {
		return slices.Contains(assistantTexts(readRecordedMessages(t, h, imported, chatID)), "OK")
	}, 10*time.Second, 5*time.Millisecond, "the replayed reply never reached the chat")

	assert.Equal(t, "app-server", l.Argv[0], "the launch is the app-server, not a TUI")
	requireOneLaunch(t, fake, l)
}

func TestFakeProvider_ACodexAddedAndDeletedFileRecordWholeFileDiffs(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "codex", Turn: []string{
		"item_started.fileChange.json", "item_completed.fileChange.json", "item_completed.fileChange.delete.json",
	}})
	chatID := createChat(t, h, imported, fake.ID)

	promptLaunch(t, h, imported, fake, chatID, "edit", "")
	diffs := map[string]string{}
	require.Eventually(t, func() bool {
		for _, row := range ledgerRows(t, h, chatID) {
			if row.Kind != ledger.KindToolCall {
				continue
			}
			var call ledger.ToolCall
			if json.Unmarshal(row.Body, &call) == nil && call.Phase == ledger.ToolPost {
				diffs[call.ToolID] = call.Diff
			}
		}
		return len(diffs) == 2
	}, 10*time.Second, 5*time.Millisecond, "the replayed edits never finished")

	added := diffs["exec-6288131f-d9af-4773-b45e-e158ea113823"]
	assert.Contains(t, added, "--- /dev/null\n+++ b/outside-cx.txt\n@@ -0,0 +1 @@\n+hi\n")
	deleted := diffs["exec-e1bfbc8c-8514-4da8-afa9-b4e8c35f3937"]
	assert.Contains(t, deleted, "--- a/ws/src/util.go\n+++ /dev/null\n@@ -1,5 +0,0 @@\n-package src\n")
}
