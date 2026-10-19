//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// livestubPromptSubmitYAML extends writeLiveStubProviderDescriptor's stub
// (agent_working_overlay_test.go) with the presentation.prompt_submit and
// session.resume blocks POST .../prompts requires and that shared descriptor
// never declares — no earlier user of createLiveStubChat submits a real
// prompt over HTTP, only postAgentHook. Without them SubmitPrompt refuses
// every text with 422 (runner.ErrPromptUnsupported), so this test carries
// its own override rather than editing the shared harness file.
//
// spawn.cmd is "sh -c cat", not bare "cat": prompt_submit's fresh/resume
// steps place {message} as a trailing positional argv entry (mirroring
// descriptors-v3/claude.yaml), and "cat -- <message>" would try to OPEN a
// file named after the message and exit immediately, breaking the "stays
// alive on its PTY" property the shared stub relies on. The extra argv lands
// on $0/$1 of the sh -c script, which the literal script "cat" never
// references, so /bin/cat still runs bare, reading stdin, alive as before.
const livestubPromptSubmitYAML = `id: livestub
spawn:
  cmd: "sh"
  args: ["-c", "cat"]
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
`

func writeLiveStubProviderDescriptorWithPromptSubmit(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "livestub.yaml"), []byte(stubDescriptor(livestubPromptSubmitYAML)), 0o644))
}

// TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue proves
// the fix for "user's turns after some time of idle is lost, and does not
// record anywhere": a submitted prompt's literal text is recoverable from
// the backend even when the client that sent it has no memory of having
// done so — not merely a hash it cannot use to show the user anything back.
//
// This is the first test in this suite to drive the real POST .../prompts
// route against a live-stub chat (createLiveStubChat) rather than a
// hooks-simulated one. See livestubPromptSubmitYAML's doc comment for what
// that turned out to require and why: the shared stub descriptor
// (writeLiveStubProviderDescriptor) cannot dispatch a prompt at all over
// this route, a 422 for any text, so this test supplies its own descriptor
// override instead.
//
// The GET below is a single, unretried read straight after the POST
// returns, with no user_prompt hook ever fired to confirm acceptance — the
// point of the test is to catch the record BEFORE anything could confirm
// it, so waiting for settlement here would defeat the assertion rather than
// stabilise it.
func TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptorWithPromptSubmit(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createLiveStubChat(t, h, imported)

	const submittedText = "please rename this function to something clearer"

	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/prompts",
		map[string]string{"text": submittedText, "clientRequestId": "11111111-1111-1111-1111-111111111111"},
		http.StatusOK,
	)
	_ = resp.Body.Close()

	var got struct {
		Text  string `json:"text"`
		State string `json:"state"`
	}
	h.get(repoBase(imported)+"/chats/"+chatID+"/pending-prompt", &got)

	assert.Equal(t, submittedText, got.Text,
		"the recovered record must carry the literal submitted text, not merely a hash — "+
			"the whole point of the fix is giving the user their words back")
	assert.Equal(t, "pending", got.State,
		"the replacement runner was spawned and its identity durably recorded, but nothing "+
			"has fired the user_prompt hook that would confirm the provider accepted it, so the "+
			"record must still read as pending — never accepted")
}

// postPrompt submits text under requestID and returns the response status.
func postPrompt(t *testing.T, h *harness, imported importedRepo, chatID, text, requestID string) int {
	t.Helper()
	body, err := json.Marshal(map[string]string{"text": text, "clientRequestId": requestID})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		h.url+repoBase(imported)+"/chats/"+chatID+"/prompts", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return statusOf(t, h, req)
}

func pendingPromptStatus(t *testing.T, h *harness, imported importedRepo, chatID string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		h.url+repoBase(imported)+"/chats/"+chatID+"/pending-prompt", http.NoBody)
	require.NoError(t, err)
	return statusOf(t, h, req)
}

func statusOf(t *testing.T, h *harness, req *http.Request) int {
	t.Helper()
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

func isPromptSettled(requestID string) func(map[string]any) bool {
	return func(m map[string]any) bool {
		return m["kind"] == "prompt_settled" && m["clientRequestId"] == requestID
	}
}

func TestRegression_APromptWhoseLaunchFailsIsSettledNotLost(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, _ := createFakeChat(t, h, imported, fake)
	fake.BreakSpawn(t)
	conn := h.dial(repoBase(imported) + "/chats/ws")
	requestID := uuid.NewString()

	status := postPrompt(t, h, imported, chatID, "a prompt no CLI will ever see", requestID)

	assert.GreaterOrEqual(t, status, http.StatusBadRequest, "a launch that failed is not a delivery")
	settled := readUntil(t, conn, isPromptSettled(requestID))
	assert.Contains(t, []any{nil, false}, settled["promptConsumed"], "nothing consumed the prompt: absent or false")
	assert.Equal(t, http.StatusNoContent, pendingPromptStatus(t, h, imported, chatID),
		"a settled prompt is not pending")
}

func TestRegression_ResendingTheSameRequestIDNeverDeliversTwice(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, _ := createFakeChat(t, h, imported, fake)
	const text = "deliver this exactly once"
	requestID := uuid.NewString()

	first := postPrompt(t, h, imported, chatID, text, requestID)
	h.QuiesceReactors()
	carrier := getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID
	second := postPrompt(t, h, imported, chatID, text, requestID)
	h.QuiesceReactors()

	assert.Equal(t, http.StatusOK, first)
	assert.Equal(t, http.StatusOK, second, "a resend of the same request is answered, not refused")
	assert.Equal(t, carrier, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID,
		"the resend must not replace the CLI carrying the prompt")
	fake.AwaitLaunch(t, carrier)
	assert.Equal(t, 1, launchesCarrying(t, fake, text))
}

// launchesCarrying counts the fake CLI launches whose argv carried text.
func launchesCarrying(t *testing.T, fake kit.FakeProvider, text string) int {
	t.Helper()
	captures, err := filepath.Glob(filepath.Join(fake.Dir, "*.json"))
	require.NoError(t, err)
	count := 0
	for _, path := range captures {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var launch kit.Launch
		require.NoError(t, json.Unmarshal(raw, &launch))
		if strings.Contains(strings.Join(launch.Argv, "\x00"), text) {
			count++
		}
	}
	return count
}

func TestRegression_APromptTheProviderNeverAcknowledgesSettlesOnIdle(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, _ := createFakeChat(t, h, imported, fake)
	hookBase := h.url + repoBase(imported)
	conn := h.dial(repoBase(imported) + "/chats/ws")
	requestID := uuid.NewString()

	require.Equal(t, http.StatusOK, postPrompt(t, h, imported, chatID, "are you there", requestID))
	h.QuiesceReactors()
	launch := fake.AwaitLaunch(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID)
	for _, step := range []struct{ event, payload string }{
		{"session_start", `{"session_id":"s-1","source":"resume"}`},
		{"idle", `{"session_id":"s-1","message":"Claude is waiting for your input"}`},
	} {
		resp := launch.PostHook(t, hookBase, "claude", step.event, []byte(step.payload)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
		require.Equal(t, http.StatusOK, resp.StatusCode, step.event)
	}

	settled := readUntil(t, conn, isPromptSettled(requestID))
	assert.Contains(t, []any{nil, false}, settled["promptConsumed"], "the provider never took the prompt: absent or false")
	assert.Equal(t, http.StatusOK, postPrompt(t, h, imported, chatID, "and now?", uuid.NewString()),
		"a settled prompt no longer holds the chat busy")
}
