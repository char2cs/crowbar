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

	"github.com/char2cs/crowbar/api/tests/kit"
)

// stubs are the stub providers installed per crowbar home, so a hook can be
// posted with the launch of the runner it names. Keyed "<home>|<id>".
var stubs sync.Map

// stubLifecycle is what a stub that declares only what its test exercises is
// completed with: the lifecycle every descriptor needs before it is enabled.
var stubLifecycle = map[string]map[string]any{
	"session_start": {"in": "session_start", "map": map[string]any{"session_id": "session_id"}},
	"user_prompt":   {"in": "user_prompt", "map": map[string]any{"message": "prompt"}},
	"turn_stop":     {"in": "turn_stop", "map": map[string]any{"session_id": "session_id", "message": "last_assistant_message"}},
}

// writeProviderDescriptor installs stub provider id over the fake CLI: body's
// blocks, with its events completed by stubLifecycle. A stub that declares its
// own spawn keeps it; one that does not runs the fake CLI, which acts as `cat`.
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

// stubLaunch is the fake CLI placed as runnerID, of stub provider id.
func stubLaunch(t *testing.T, h *harness, id, runnerID string) kit.Launch {
	t.Helper()
	stored, _ := stubs.Load(h.home + "|" + id)
	fake, ok := stored.(kit.FakeProvider)
	require.True(t, ok, "stub provider %q was never installed", id)
	return fake.AwaitLaunch(t, runnerID)
}

// postHook posts one hook of runnerID's CLI through the hook route under base
// (a repo or home route prefix), and requires it accepted.
func postHook(t *testing.T, h *harness, base, provider, runnerID, event, payload string) []byte {
	t.Helper()
	resp := stubLaunch(t, h, provider, runnerID).PostHook(t, h.url+base, provider, event, []byte(payload)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", event, body)
	return body
}

// heldHook is how a hook the daemon held open ended: its status and body, or
// the error its request failed with.
type heldHook struct {
	status int
	body   string
	err    error
}

// askHook posts a hook that opens an ask. The daemon holds it open until the
// ask is decided, so it is answered on the channel; ending ctx hangs it up.
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

// heldChoice is the id of the ask a held hook opened on chatID: the first
// answerable one not in seen.
func heldChoice(t *testing.T, h *harness, base, chatID string, seen ...string) string {
	t.Helper()
	var id string
	require.Eventually(t, func() bool {
		var choices []struct {
			ID         string `json:"id"`
			Answerable bool   `json:"answerable"`
		}
		h.get(base+"/chats/"+chatID+"/choices", &choices)
		for _, c := range choices {
			if c.Answerable && !slices.Contains(seen, c.ID) {
				id = c.ID
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond, "no held ask opened on chat %s", chatID)
	return id
}

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
	chatID, launch = createFakeChat(t, h, imported, fake)
	return chatID, launch, h.url + repoBase(imported)
}

// createFakeChat creates a chat on an installed fake provider and returns it
// with the launch of the CLI placed on it.
func createFakeChat(t *testing.T, h *harness, imported importedRepo, fake kit.FakeProvider) (string, kit.Launch) {
	t.Helper()
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
	return created.ID, fake.AwaitLaunch(t, detail.LiveRunnerID)
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
