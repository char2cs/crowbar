package apidriver_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// leadDescriptor opens sessions with no field for instructions: the document can only lead
// a prompt's text.
func leadDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	d, err := descriptor.ParseV0([]byte(`
id: neutral
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [x]
    handshake: { call: hello, notify: ready }
  spawn:
    cmd: x
events:
  prompt:
    fresh:
      - { call: open, capture: { session_id: id } }
    resume:
      - { call: reopen, send: { session: "{session_id}" } }
    action:
      - { call: submit, send: { session: "{session_id}", text: "{text}" } }
inject:
  - at: context
    prompt_lead: "<record>{context}</record>"
`))
	require.NoError(t, err)
	return d
}

func sentText(t *testing.T, params string) string {
	t.Helper()
	var p struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(params), &p))
	return p.Text
}

// A new session's first prompt is led by the document its creating call had no field for,
// and only that one.
func TestDispatch_ANewSessionsFirstPromptCarriesTheContextLead(t *testing.T) {
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "open", result: `{"id":"s1"}`},
		{method: "submit", result: `{}`},
		{method: "submit", result: `{}`},
	})
	drv, err := apidriver.Start(t.Context(), leadDescriptor(t), stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })
	_, err = drv.EstablishSession(t.Context(), "prompt", map[string]string{"context": "the record\n"})
	require.NoError(t, err)

	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "one"})
	require.NoError(t, err)
	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "two"})
	require.NoError(t, err)

	assert.Equal(t, "<record>the record\n</record>\n\none", sentText(t, (*seen)[1]))
	assert.Equal(t, "two", sentText(t, (*seen)[2]), "the document rides one prompt only")
}

// A resumed session has its own history; only a gap Crowbar owes it is handed over, with
// no call of its own, on the next prompt.
func TestInjectAt_APromptLeadMakesNoCallAndRidesTheNextPrompt(t *testing.T) {
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "reopen", result: `{}`},
		{method: "submit", result: `{}`},
	})
	drv, err := apidriver.Start(t.Context(), leadDescriptor(t), stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })
	_, err = drv.EstablishSession(t.Context(), "prompt", map[string]string{"session_id": "s1", "context": "ignored"})
	require.NoError(t, err)

	require.NoError(t, drv.InjectAt(t.Context(), "context", map[string]string{"session_id": "s1", "context": "the gap"}))
	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "hello"})
	require.NoError(t, err)

	require.Len(t, *seen, 2, "the lead is not a call of its own")
	assert.Equal(t, "<record>the gap</record>\n\nhello", sentText(t, (*seen)[1]))
}

// A resume with nothing owed leads nothing: the lead is queued by a fresh session or an
// explicit inject, never by the resume itself.
func TestDispatch_AResumedSessionCarriesNoLeadUnlessInjected(t *testing.T) {
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "reopen", result: `{}`},
		{method: "submit", result: `{}`},
	})
	drv, err := apidriver.Start(t.Context(), leadDescriptor(t), stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })

	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"session_id": "s1", "context": "the record", "text": "hello"})
	require.NoError(t, err)

	assert.Equal(t, "hello", sentText(t, (*seen)[1]))
}

// A prompt the provider refuses has not been led: the document waits for the one it takes.
func TestDispatch_ARefusedPromptKeepsTheLeadForTheNext(t *testing.T) {
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "open", result: `{"id":"s1"}`},
		{method: "submit", errCode: 1, errMessage: "busy"},
		{method: "submit", result: `{}`},
	})
	drv, err := apidriver.Start(t.Context(), leadDescriptor(t), stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })
	_, err = drv.EstablishSession(t.Context(), "prompt", map[string]string{"context": "the record"})
	require.NoError(t, err)

	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "one"})
	require.Error(t, err)
	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "again"})
	require.NoError(t, err)

	assert.Equal(t, "<record>the record</record>\n\nagain", sentText(t, (*seen)[2]))
}
