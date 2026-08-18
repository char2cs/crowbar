//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answerStubProviderDescriptorYAML is livestub plus the two blocks that make a
// prompt ANSWERABLE from Crowbar: a permission mapping to observe one, and an
// `answer:` block declaring what to print when a human decides.
//
// It is a separate descriptor rather than a change to livestub for the same
// reason memstub is: livestub's job in the other agent tests is to be the
// provider that declares nothing, which is the fixture proving this whole channel
// is opt-in.
//
// The budget is seconds rather than claude's 270 so a broken await FAILS rather
// than parks — nothing in this test is supposed to reach the deadline.
const answerStubProviderDescriptorYAML = `id: answerstub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
    permission:
      session_id: session_id
      prompt_id:  prompt_id
      message:    tool_name
      tool_name:  tool_name
      tool_target: tool_input.command
      tool_input: tool_input
answer:
  permission:
    timeout_seconds: 5
    responses:
      allow: '{"decision":{"behavior":"allow"}}'
      deny: '{"decision":{"behavior":"deny","message":{reason_json}}}'
`

func writeAnswerStubProviderDescriptor(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "answerstub.yaml"), []byte(answerStubProviderDescriptorYAML), 0o644))
}

// TestRegression_AnAnswerInTheAckAwaitWindowStillReachesTheRelay drives the whole
// answer channel over HTTP, in the order a real relay produces it.
//
// The relay is a SEPARATE PROCESS making TWO round trips: it POSTs the hook, the
// daemon acks with a stay-alive directive, and only then does it POST
// /hooks/await. Between those two calls there is a real window — the relay is
// still draining the rest of its FIFO spool — and a fast human answers inside it.
//
// Before verdicts were retained, that answer resolved the prompt and took the
// slot off the desk, so the await landing a moment later found nothing and
// returned an empty stdout. The user watched their answer be accepted while the
// CLI never received a byte and sat on a dialog nobody could clear. The record
// lied, which is worse than a prompt that was never answerable at all.
//
// Every step here is the production wire shape, taken from cmd/crowbar's relay.
func TestRegression_AnAnswerInTheAckAwaitWindowStillReachesTheRelay(t *testing.T) {
	h := newHarness(t)
	writeAnswerStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := wsBase(imported)

	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/agent/chats", map[string]string{"provider": "answerstub"},
		http.StatusCreated, &created)
	h.Quiesce()
	segID := getAgentChat(t, h, base, created.ID).LiveRunnerID
	require.NotEmpty(t, segID)

	// A turn has to be open, or the permission is recorded already-resolved: a
	// prompt over an idle agent is a banner nothing clears.
	postAnswerStubHook(t, h, base, segID, "", "user_prompt", `{"prompt":"go"}`)
	h.Quiesce()

	// The hook the CLI is blocked on. The ack carries the ONE instruction this
	// channel adds: stay alive, a human is being asked.
	// The delivery id is the relay's own, and the daemon requires a canonical UUID:
	// it is the journal key that makes a retried hook ONE semantic delivery.
	deliveryID := uuid.NewString()
	ack := postAnswerStubHook(t, h, base, segID, deliveryID, "permission",
		`{"session_id":"s1","prompt_id":"p1","tool_name":"Bash",`+
			`"tool_input":{"command":"touch PROOF"}}`)
	require.NotNil(t, ack.Await, "a prompt this provider can be told the answer to holds its relay")
	assert.Positive(t, ack.Await.WaitMS, "the relay is given a finite budget, never an open one")
	h.Quiesce()

	var choices []struct {
		ID      string `json:"id"`
		Options []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"options"`
		Answerable bool `json:"answerable"`
	}
	h.get(base+"/agent/chats/"+created.ID+"/choices", &choices)
	require.Len(t, choices, 1)
	require.Equal(t, ack.Await.ChoiceID, choices[0].ID)
	allow := ""
	for _, option := range choices[0].Options {
		if option.Kind == "allow" {
			allow = option.ID
		}
	}
	require.NotEmpty(t, allow, "a permission offers an allow")

	// THE WINDOW. The human answers with the relay's await POST still in flight —
	// nothing is parked on this prompt at this instant.
	h.post(base+"/agent/chats/"+created.ID+"/choices/"+choices[0].ID+"/answer",
		map[string]any{"optionIds": []string{allow}}, http.StatusOK, nil)
	h.Quiesce()

	// The relay's long-poll finally lands, and must be handed the decision made
	// before it asked.
	var answer struct {
		Stdout string `json:"stdout"`
	}
	h.post(base+"/agent/hooks/await", map[string]string{"delivery_id": deliveryID},
		http.StatusOK, &answer)
	assert.JSONEq(t, `{"decision":{"behavior":"allow"}}`, answer.Stdout,
		"the CLI must receive the bytes Crowbar's record says it was given")

	// And exactly once. A retained verdict is not a mailbox: a second poll on that
	// delivery prints nothing, or the provider runs the gated tool twice.
	var second struct {
		Stdout string `json:"stdout"`
	}
	h.post(base+"/agent/hooks/await", map[string]string{"delivery_id": deliveryID},
		http.StatusOK, &second)
	assert.Empty(t, second.Stdout, "a claimed verdict is gone")
}

// answerStubAck mirrors dto.AgentHookAckDTO: the acknowledgement a relay reads
// its ONE possible instruction off — stay alive, a human is being asked.
type answerStubAck struct {
	Await *struct {
		ChoiceID string `json:"choiceId"`
		WaitMS   int64  `json:"waitMs"`
	} `json:"await"`
}

// postAnswerStubHook forwards a raw hook payload exactly as the in-PTY `crowbar
// hook` relay does (body shape from cmd/crowbar/hook.go's runHook) and returns
// the acknowledgement.
//
// An EMPTY body is read as "no directive", which is what the relay itself does:
// a hook that opened nothing answerable gets a bare 202 and exits.
func postAnswerStubHook(
	t *testing.T,
	h *harness,
	base, segID, deliveryID, event, payloadRaw string,
) answerStubAck {
	t.Helper()
	resp := h.raw(http.MethodPost, base+"/agent/hooks", map[string]string{
		"delivery_id": deliveryID,
		"segment_id":  segID,
		"provider":    "answerstub",
		"event":       event,
		"payload_raw": payloadRaw,
	}, http.StatusAccepted)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	if len(bytes.TrimSpace(body)) == 0 {
		return answerStubAck{}
	}
	var envelope struct {
		Data answerStubAck `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope.Data
}
