package apidriver_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestSend_ACapturedNumberKeepsItsJSONType(t *testing.T) {
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
      - call: open
        capture: { session_id: id }
    action:
      - call: submit
        send: { session: "{session_id}", text: "{text}" }
        capture: { fence: generation, live: active }
  interrupt:
    out: cancel
    send: { session: "{session_id}", fence: "{fence}", live: "{live}", tag: "g{fence}", retries: 2 }
`))
	require.NoError(t, err)
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "open", result: `{"id":"12345"}`},
		{method: "submit", result: `{"generation":7,"active":true}`},
		{method: "cancel", result: `{}`},
	})
	drv, err := apidriver.Start(t.Context(), d, stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })

	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "hi"})
	require.NoError(t, err)
	require.NoError(t, drv.Send(t.Context(), "interrupt", nil))

	require.JSONEq(t, `{"session":"12345","fence":7,"live":true,"tag":"g7","retries":2}`, (*seen)[2])
}

func TestEstablishSession_AnOptionalSendKeyIsOmittedWhileEmpty(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	fresh := &d.Events["prompt"].Fresh[0]
	fresh.Send["pick"] = "{model.name}"
	fresh.Optional = []string{"pick"}

	for name, tc := range map[string]struct {
		values map[string]string
		want   string
	}{
		"empty":   {map[string]string{"cwd": "/work"}, ""},
		"present": {map[string]string{"cwd": "/work", "model.name": "m1"}, `"pick":"m1"`},
	} {
		t.Run(name, func(t *testing.T) {
			stdio, seen := scriptedServer(t, []scriptedCall{{method: "thread/start", result: `{"thread":{"id":"t-1"}}`}})
			ctx := t.Context()
			drv, err := apidriver.Start(ctx, d, stdio, nil)
			require.NoError(t, err)
			defer func() { _ = drv.Close() }()

			_, err = drv.EstablishSession(ctx, "prompt", tc.values)

			require.NoError(t, err)
			if tc.want == "" {
				require.NotContains(t, (*seen)[0], `"pick"`)
				return
			}
			require.Contains(t, (*seen)[0], tc.want)
		})
	}
}

func TestEstablishSession_TheShippedGatewayRefusalIsALostSession(t *testing.T) {
	for name, tc := range map[string]struct {
		refusal scriptedCall
		lost    bool
	}{
		"the pinned-block refusal": {scriptedCall{method: "session.resume", errCode: 4001, errMessage: "acp_mcp_policy_conflict"}, true},
		"another error, same code": {scriptedCall{method: "session.resume", errCode: 4001, errMessage: "invalid params"}, false},
		"the reason, another code": {scriptedCall{method: "session.resume", errCode: -32603, errMessage: "acp_mcp_policy_conflict"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../descriptor/descriptors-v3/hermes.yaml")
			require.NoError(t, err)
			d, err := descriptor.ParseV0(raw)
			require.NoError(t, err)
			d.Runtime.API.Handshake.Notify = "ready"
			stdio, _ := scriptedServer(t, []scriptedCall{tc.refusal})
			drv, err := apidriver.Start(t.Context(), d, stdio, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = drv.Close() })

			_, err = drv.EstablishSession(t.Context(), "prompt", map[string]string{"session_id": "s-1"})

			require.Error(t, err)
			assert.Equal(t, tc.lost, errors.Is(err, apidriver.ErrSessionLost), err.Error())
		})
	}
}

func TestReconnect_ARememberedFrameValueDoesNotOutliveTheConnection(t *testing.T) {
	first, second := newLink(), newLink()
	d := reconnectDescriptor()
	d.Runtime.API.Remember = []spec.FrameCapture{{In: spec.WireRef{"note"}, Capture: map[string]string{"fence": "gen"}}}
	d.Events["interrupt"] = spec.EventSpec{Out: spec.WireRef{"cancel"}, Send: map[string]any{"id": "{captured.fence}"}}
	rc := apidriver.Reconnect{Max: 3, Backoff: time.Second, Sleep: instantSleep(make(chan time.Duration, 4))}
	drv, err := apidriver.StartDial(t.Context(), d, dialer(first, second), rc, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })
	handshakeCalls(t, first)
	_, err = drv.EstablishSession(t.Context(), "prompt", nil)
	require.NoError(t, err)
	first.next(t, "open:")
	first.in <- []byte(`{"method":"note","params":{"gen":"g3"}}`)
	require.Equal(t, "notification", nextEvent(t, drv).Canonical)

	first.drop()
	handshakeCalls(t, second)
	second.next(t, "enter:s1")
	require.NoError(t, drv.Send(t.Context(), "interrupt", nil))

	second.next(t, "cancel:")
}
