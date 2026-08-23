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

const answerStubProviderDescriptorYAML = `id: answerstub
spawn:
  cmd: "cat"
  interactive_required: true
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
  permission:
    ask: permission
    timeout_seconds: 5
    map:
      session_id: session_id
      prompt_id:  prompt_id
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

func writeAnswerStubProviderDescriptor(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "answerstub.yaml"), []byte(answerStubProviderDescriptorYAML), 0o644,
	))
}

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

	postAnswerStubHook(t, h, base, segID, "", "user_prompt", `{"prompt":"go"}`)
	h.Quiesce()

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

	h.post(base+"/agent/chats/"+created.ID+"/choices/"+choices[0].ID+"/answer",
		map[string]any{"optionIds": []string{allow}}, http.StatusOK, nil)
	h.Quiesce()

	var answer struct {
		Stdout string `json:"stdout"`
	}
	h.post(base+"/agent/hooks/await", map[string]string{"delivery_id": deliveryID},
		http.StatusOK, &answer)
	assert.JSONEq(t, `{"decision":{"behavior":"allow"}}`, answer.Stdout,
		"the CLI must receive the bytes Crowbar's record says it was given")

	var second struct {
		Stdout string `json:"stdout"`
	}
	h.post(base+"/agent/hooks/await", map[string]string{"delivery_id": deliveryID},
		http.StatusOK, &second)
	assert.Empty(t, second.Stdout, "a claimed verdict is gone")
}

type answerStubAck struct {
	Await *struct {
		ChoiceID string `json:"choiceId"`
		WaitMS   int64  `json:"waitMs"`
	} `json:"await"`
}

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
