//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// livestubPromptSubmitYAML extends writeLiveStubProviderDescriptor's stub
// (agent_working_overlay_test.go) with the presentation.prompt_submit and
// session.resume blocks POST .../prompts requires and that shared descriptor
// never declares — no earlier user of createLiveStubChat submits a real
// prompt over HTTP, only hooks. Without them SubmitPrompt refuses every text
// with 422 (runner.ErrPromptUnsupported), so this test carries its own
// override rather than editing the shared harness file. The fake CLI ignores
// the trailing {message} positional prompt_submit places, staying alive.
const livestubPromptSubmitYAML = `id: livestub
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
	writeProviderDescriptor(t, h, "livestub", livestubPromptSubmitYAML)
}

// A submitted prompt's literal text is on the chat's record as pending the
// moment the POST returns, before any hook could confirm it, so a client that
// lost its own copy gets the words back. Read once, unretried, on purpose.
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

	pending := chatState(t, h, chatID).PendingPrompts()
	require.Len(t, pending, 1)
	assert.Equal(t, submittedText, pending[0].Text, "the record carries the literal submitted text")
	assert.Equal(t, ledger.PromptStatusPending, pending[0].Status,
		"nothing confirmed the provider took it, so it still reads as pending")
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

func statusOf(t *testing.T, h *harness, req *http.Request) int {
	t.Helper()
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

// untilSettled waits for requestID's prompt_settled row on chatID's feed.
func untilSettled(t *testing.T, h *harness, chatID, requestID string) ledger.PromptSettled {
	t.Helper()
	var settled []ledger.PromptSettled
	require.Eventually(t, func() bool {
		settled = settledAs(t, chatRows(t, h, chatID), requestID)
		return len(settled) > 0
	}, 10*time.Second, 10*time.Millisecond, "the prompt was never settled")
	return settled[0]
}

func TestRegression_APromptWhoseLaunchFailsIsSettledNotLost(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, _ := createFakeChat(t, h, imported, fake)
	fake.BreakSpawn(t)
	requestID := uuid.NewString()

	status := postPrompt(t, h, imported, chatID, "a prompt no CLI will ever see", requestID)

	assert.GreaterOrEqual(t, status, http.StatusBadRequest, "a launch that failed is not a delivery")
	assert.False(t, untilSettled(t, h, chatID, requestID).Consumed, "nothing consumed the prompt")
	assert.Empty(t, chatState(t, h, chatID).PendingPrompts(), "a settled prompt is not pending")
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

	assert.False(t, untilSettled(t, h, chatID, requestID).Consumed, "the provider never took the prompt")
	assert.Equal(t, http.StatusOK, postPrompt(t, h, imported, chatID, "and now?", uuid.NewString()),
		"a settled prompt no longer holds the chat busy")
}

// Two sends racing on a chat whose idle CLI the first must replace: whichever
// the daemon refuses as busy left no trace, and the one it accepted is still
// owed its delivery. The race is the order the two retake the chat after that
// CLI quits, so a few chats run it.
func TestRegression_TwoRacingSendsNeverLoseTheAcceptedOne(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	for range 6 {
		chatID, _ := createFakeChat(t, h, imported, fake)
		ids := []string{uuid.NewString(), uuid.NewString()}
		statuses := racePrompts(t, h, imported, chatID, ids)
		h.QuiesceReactors()
		st := chatState(t, h, chatID)
		for i, id := range ids {
			p, recorded := st.Receipt(id)
			if statuses[i] == http.StatusConflict {
				assert.False(t, recorded, "a send answered busy must not be on the record")
				continue
			}
			require.Equal(t, http.StatusOK, statuses[i])
			require.True(t, recorded)
			assert.NotEqual(t, ledger.PromptStatusSettled, p.Status,
				"an accepted send was settled %q behind its client's back", p.Reason)
		}
	}
}

// racePrompts posts one prompt per request id at once and returns each status.
func racePrompts(t *testing.T, h *harness, imported importedRepo, chatID string, ids []string) []int {
	t.Helper()
	reqs := make([]*http.Request, len(ids))
	for i, id := range ids {
		body, err := json.Marshal(map[string]string{"text": "send " + id, "clientRequestId": id})
		require.NoError(t, err)
		reqs[i], err = http.NewRequestWithContext(t.Context(), http.MethodPost,
			h.url+repoBase(imported)+"/chats/"+chatID+"/prompts", bytes.NewReader(body))
		require.NoError(t, err)
		reqs[i].Header.Set("Content-Type", "application/json")
	}
	statuses, errs := make([]int, len(ids)), make([]error, len(ids))
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Go(func() {
			resp, err := h.server.Client().Do(req)
			if errs[i] = err; err == nil {
				statuses[i], errs[i] = resp.StatusCode, resp.Body.Close()
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	return statuses
}
