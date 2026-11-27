//go:build integration

package tests

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

const steerStubDescriptorYAML = `id: %s
spawn:
  cmd: "sh"
  args: ["-c", "echo launched >> %s; exec '%s'"]
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
events:
  session_start:
    in: session_start
    map: { session_id: session_id, model: model }
  user_prompt:
    in: user_prompt
    map: { message: prompt }
  turn_stop:
    in: turn_stop
    map: { session_id: session_id, message: last_assistant_message }
  turn_failed:
    in: turn_failed
    required: [session_id, reason]
    map: { session_id: session_id, reason: error }
runtime:
  transport: hooks
  hooks:
    format: json
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low, high]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    steer:
      skip_prefixes: ["/"]
      frame: "STEERED: {message}"
      reply: '{"decision":"block","reason":{message_json}}'
`

type steerFixture struct {
	h        *harness
	imported importedRepo
	chatID   string
	runnerID string
	launches string
	provider string
}

func newSteerFixture(t *testing.T) steerFixture {
	t.Helper()
	h := newHarness(t)
	f := steerFixture{h: h, provider: "steerstub", launches: filepath.Join(t.TempDir(), "launches")}
	writeProviderDescriptor(t, h, f.provider, fmt.Sprintf(steerStubDescriptorYAML, f.provider, f.launches, kit.FakeCLIPath(t)))
	f.imported = importWritableWorkspace(t, h)
	f.chatID, f.runnerID = createStubChat(t, h, f.imported, f.provider)
	require.Eventually(t, f.launchedTimes(1), 10*time.Second, 5*time.Millisecond)
	return f
}

func (f steerFixture) launchCount(t *testing.T) int {
	t.Helper()
	n, err := f.launched()
	require.NoError(t, err)
	return n
}

func (f steerFixture) launched() (int, error) {
	raw, err := os.ReadFile(f.launches)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return strings.Count(string(raw), "launched"), err
}

func (f steerFixture) launchedTimes(n int) func() bool {
	return func() bool {
		got, err := f.launched()
		return err == nil && got == n
	}
}

func (f steerFixture) hook(t *testing.T, event, payload string) string {
	t.Helper()
	return string(postHook(t, f.h, repoBase(f.imported), f.provider, f.runnerID, event, payload))
}

func (f steerFixture) submit(t *testing.T, text, requestID string, want int) {
	t.Helper()
	f.submitBody(t, map[string]string{"text": text, "clientRequestId": requestID}, want)
}

func (f steerFixture) submitBody(t *testing.T, body map[string]string, want int) {
	t.Helper()
	_ = f.h.raw(http.MethodPost, repoBase(f.imported)+"/chats/"+f.chatID+"/prompts", body, want).Body.Close()
}

func (f steerFixture) pendingState(t *testing.T) string {
	t.Helper()
	pending := chatState(t, f.h, f.chatID).PendingPrompts()
	if len(pending) == 0 {
		return ""
	}
	return string(pending[0].Status)
}

func (f steerFixture) liveRunner(t *testing.T) string {
	t.Helper()
	f.h.Quiesce()
	return getAgentChat(t, f.h, repoBase(f.imported), f.chatID).LiveRunnerID
}

func TestRegression_PromptSentMidTurnReachesTheRunningCLI(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)
	requestID := uuid.NewString()

	f.submit(t, "also check the tests", requestID, http.StatusOK)

	assert.Equal(t, "pending", f.pendingState(t))
	assert.Equal(t, f.runnerID, f.liveRunner(t), "the monitored CLI must not be replaced")
	assert.Equal(t, 1, f.launchCount(t), "the CLI must be launched exactly once")

	reply := f.hook(t, "turn_stop", `{"session_id":"sid-1","last_assistant_message":"first done"}`)

	assert.JSONEq(t, `{"decision":"block","reason":"STEERED: also check the tests"}`, reply,
		"the turn-end hook must answer with the parked message")
	assert.Equal(t, f.runnerID, f.liveRunner(t))
	assert.Equal(t, 1, f.launchCount(t))
	assert.Empty(t, f.pendingState(t), "the delivery is proven accepted once the user turn is recorded")

	f.hook(t, "turn_stop", `{"session_id":"sid-1","last_assistant_message":"tests checked"}`)
	f.h.Quiesce()
	var order []string
	for _, m := range readRecordedMessages(t, f.h, f.imported, f.chatID) {
		order = append(order, m.Role+":"+m.Text)
	}
	assert.Equal(t, []string{
		"user:first", "assistant:first done", "user:also check the tests", "assistant:tests checked",
	}, order, "the steered message appears exactly once, between the reply it followed and the one it caused")
}

func TestRegression_SteeredPromptIsRefusedWhenTheTurnFails(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusOK)

	reply := f.hook(t, "turn_failed", `{"session_id":"sid-1","error":"overloaded"}`)

	assert.Empty(t, reply)
	assert.Empty(t, f.pendingState(t), "a refused delivery is over, not pending")
	f.submit(t, "retry after failure", uuid.NewString(), http.StatusOK)
}

func TestRegression_CommandSentMidTurnIsNotSteered(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)

	f.submit(t, "/compact", uuid.NewString(), http.StatusConflict)

	assert.Equal(t, 1, f.launchCount(t))
}

func TestRegression_PromptSentWhileIdleStillRestartsTheCLI(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)

	f.submit(t, "hello", uuid.NewString(), http.StatusOK)

	require.Eventually(t, f.launchedTimes(2), 10*time.Second, 5*time.Millisecond)
}

func TestRegression_StopRefusesAParkedPromptInsteadOfWedgingTheChat(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusOK)

	_ = f.h.raw(http.MethodPost, repoBase(f.imported)+"/chats/"+f.chatID+"/stop", nil, http.StatusNoContent).Body.Close()

	assert.Empty(t, f.pendingState(t), "a refused delivery is over, not pending")
}

func TestRegression_ComposerBodyWithSelectionIsSteeredMidTurn(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1","model":"stub-sonnet-resolved-1"}`)
	picked := map[string]string{"provider": f.provider, "model": "sonnet", "effort": "low"}
	f.submitBody(t, map[string]string{
		"text": "idle send", "clientRequestId": uuid.NewString(),
		"provider": picked["provider"], "model": picked["model"], "effort": picked["effort"],
	}, http.StatusOK)
	f.runnerID = f.liveRunner(t)
	f.hook(t, "session_start", `{"session_id":"sid-1","model":"stub-sonnet-resolved-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"idle send"}`)

	f.submitBody(t, map[string]string{
		"text": "also check the tests", "clientRequestId": uuid.NewString(),
		"provider": picked["provider"], "model": picked["model"], "effort": picked["effort"],
	}, http.StatusOK)

	assert.Equal(t, f.runnerID, f.liveRunner(t), "the monitored CLI must not be replaced")
	reply := f.hook(t, "turn_stop", `{"session_id":"sid-1","last_assistant_message":"done"}`)
	assert.JSONEq(t, `{"decision":"block","reason":"STEERED: also check the tests"}`, reply)

	f.submitBody(t, map[string]string{
		"text": "switch model", "clientRequestId": uuid.NewString(),
		"provider": picked["provider"], "model": "opus", "effort": "low",
	}, http.StatusConflict)
}

func TestRegression_SelectionChangedMidTurnThroughPatchIsNotSteered(t *testing.T) {
	f := newSteerFixture(t)
	f.submitBody(t, map[string]string{
		"text": "idle send", "clientRequestId": uuid.NewString(),
		"provider": f.provider, "model": "sonnet", "effort": "low",
	}, http.StatusOK)
	f.runnerID = f.liveRunner(t)
	f.hook(t, "session_start", `{"session_id":"sid-1","model":"stub-sonnet-resolved-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"idle send"}`)

	_ = f.h.raw(http.MethodPatch, repoBase(f.imported)+"/chats/"+f.chatID+"/selection",
		map[string]string{"model": "opus", "effort": "low"}, http.StatusNoContent).Body.Close()
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusConflict)

	assert.Equal(t, f.runnerID, f.liveRunner(t), "a refused send must leave the running CLI alone")

	_ = f.h.raw(http.MethodPatch, repoBase(f.imported)+"/chats/"+f.chatID+"/selection",
		map[string]string{"model": "sonnet", "effort": "low"}, http.StatusNoContent).Body.Close()
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusOK)
	assert.Equal(t, f.runnerID, f.liveRunner(t), "back on the launched selection, the send rides the turn")
}
