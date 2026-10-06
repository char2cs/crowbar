//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
)

// steerStubDescriptorYAML is a hooks-only CLI that appends a line to a launch
// log every time it starts, so a respawn is countable, and declares steering.
const steerStubDescriptorYAML = `id: %s
spawn:
  cmd: "sh"
  args: ["-c", "echo launched >> %s; exec cat"]
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
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
	writeProviderDescriptor(t, h, f.provider, fmt.Sprintf(steerStubDescriptorYAML, f.provider, f.launches))
	f.imported = importWritableWorkspace(t, h)
	f.chatID, f.runnerID = createStubChat(t, h, f.imported, f.provider)
	return f
}

func (f steerFixture) launchCount(t *testing.T) int {
	t.Helper()
	raw, err := os.ReadFile(f.launches)
	require.NoError(t, err)
	return strings.Count(string(raw), "launched")
}

// hook relays one provider hook with a delivery id and returns the daemon's
// reply directive, the stdout the relay prints for it.
func (f steerFixture) hook(t *testing.T, event, payload string) string {
	t.Helper()
	resp := f.h.raw(http.MethodPost, repoBase(f.imported)+"/chats/hooks", map[string]string{
		"segment_id": f.runnerID, "provider": f.provider, "event": event,
		"payload_raw": payload, "delivery_id": uuid.NewString(),
	}, http.StatusAccepted)
	defer func() { _ = resp.Body.Close() }()
	var ack struct {
		Data struct {
			Reply string `json:"reply"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	return ack.Data.Reply
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
	resp, err := f.h.server.Client().Get(f.h.url + repoBase(f.imported) + "/chats/" + f.chatID + "/pending-prompt")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return ""
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got.Data.State
}

func (f steerFixture) liveRunner(t *testing.T) string {
	t.Helper()
	f.h.Quiesce()
	return getAgentChat(t, f.h, repoBase(f.imported), f.chatID).LiveRunnerID
}

// TestRegression_PromptSentMidTurnReachesTheRunningCLI is the reported "we
// cannot send messages while the agent is working": a prompt sent during a
// turn used to wait for the turn to end and then QUIT the CLI to respawn it
// with the text in argv. It must instead ride the turn-end hook into the SAME
// process, and land in the ledger once, after the reply it followed.
func TestRegression_PromptSentMidTurnReachesTheRunningCLI(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)
	requestID := uuid.NewString()

	f.submit(t, "also check the tests", requestID, http.StatusOK)

	assert.Equal(t, agentjournal.PromptStateSpawned, f.pendingState(t))
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
	for _, m := range readOrderedMessages(t, f.h, f.imported, f.chatID) {
		order = append(order, m.Role+":"+m.Text)
	}
	assert.Equal(t, []string{
		"user:first", "assistant:first done", "user:also check the tests", "assistant:tests checked",
	}, order, "the steered message appears exactly once, between the reply it followed and the one it caused")
}

// A turn that ends without its end hook ever reaching the daemon strands the
// parked message; it must be failed (so the client keeps its text and the chat
// is not left busy), never delivered into the next turn.
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

// Commands are not messages: a slash command mid-turn stays busy as before.
func TestRegression_CommandSentMidTurnIsNotSteered(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)

	f.submit(t, "/compact", uuid.NewString(), http.StatusConflict)

	assert.Equal(t, 1, f.launchCount(t))
}

// An idle chat still restarts the CLI with the prompt in argv.
func TestRegression_PromptSentWhileIdleStillRestartsTheCLI(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)

	f.submit(t, "hello", uuid.NewString(), http.StatusOK)

	assert.Equal(t, 2, f.launchCount(t))
}

// Stop retires the runner; the message parked on its turn was never delivered
// and must not leave the chat busy or be replayed into a later turn.
func TestRegression_StopRefusesAParkedPromptInsteadOfWedgingTheChat(t *testing.T) {
	f := newSteerFixture(t)
	f.hook(t, "session_start", `{"session_id":"sid-1"}`)
	f.hook(t, "user_prompt", `{"prompt":"first"}`)
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusOK)

	_ = f.h.raw(http.MethodPost, repoBase(f.imported)+"/chats/"+f.chatID+"/stop", nil, http.StatusAccepted).Body.Close()

	assert.Empty(t, f.pendingState(t), "a refused delivery is over, not pending")
}

// The composer posts the provider, model and effort picked in its selector with
// EVERY prompt, and the CLI reports its own resolved model id on session_start.
// Neither is a request to change anything mid-turn, so the message must still
// ride the running turn; only a pick that differs from the chat's own stays busy.
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

// A selection changed through the standalone PATCH route is not a request the
// running process can honour: the next send must restart it, never steer into
// the model it was launched with.
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
		map[string]string{"model": "opus", "effort": "low"}, http.StatusAccepted).Body.Close()
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusConflict)

	assert.Equal(t, f.runnerID, f.liveRunner(t), "a refused send must leave the running CLI alone")

	_ = f.h.raw(http.MethodPatch, repoBase(f.imported)+"/chats/"+f.chatID+"/selection",
		map[string]string{"model": "sonnet", "effort": "low"}, http.StatusAccepted).Body.Close()
	f.submit(t, "also check the tests", uuid.NewString(), http.StatusOK)
	assert.Equal(t, f.runnerID, f.liveRunner(t), "back on the launched selection, the send rides the turn")
}
