package apidriver_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/httpdriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func httpWireDescriptor() *spec.Descriptor {
	return &spec.Descriptor{
		ID:      "web",
		Runtime: spec.RuntimeSpec{API: spec.APISpec{Delivery: spec.APIDeliveryHTTP}},
		Events: map[string]spec.EventSpec{
			"prompt": {
				Fresh:  []spec.CallStep{{Call: "create", Capture: map[string]string{"session_id": "id"}}},
				Action: []spec.CallStep{{Call: "say", Send: map[string]any{"text": "{text}"}}},
			},
			"interrupt": {Out: spec.WireRef{"cancel"}},
			"turn_stop": {API: &spec.ChannelBlock{In: spec.WireRef{"done"}}},
			"permission": {
				ReplyRequest: "answer",
				API: &spec.ChannelBlock{
					Ask: spec.WireRef{"approval"}, Map: spec.FieldMap{"ask_id": mapping.Paths("approval_id")},
				},
			},
		},
	}
}

func TestStartHTTP_RunsStepsAndAnswersAnAskThroughItsReplyRequest(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/runs", 200, `{"id":"r1"}`)
	srv.Route("POST", "/runs/r1/say", 200, `{}`)
	srv.Route("POST", "/runs/r1/cancel", 200, `{}`)
	srv.Route("POST", "/runs/r1/answers/a7", 200, `{}`)
	srv.StreamAt("/runs/r1/events")
	client := httpdriver.New(httpdriver.Config{
		BaseURL: srv.URL, StreamPath: "/runs/{run_id}/events",
		Requests: map[string]httpdriver.Request{
			"create": {Method: "POST", Path: "/runs", Capture: map[string]string{"run_id": "id"}},
			"say":    {Method: "POST", Path: "/runs/{run_id}/say", Body: map[string]any{"text": "{text}"}},
			"cancel": {Method: "POST", Path: "/runs/{run_id}/cancel"},
			"answer": {Method: "POST", Path: "/runs/{run_id}/answers/{ask_id}", Body: map[string]any{"decision": "{decision}"}},
		},
	})
	drv, err := apidriver.StartHTTP(t.Context(), httpWireDescriptor(), client, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })

	out, err := drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "hi"})
	require.NoError(t, err)
	require.Equal(t, "r1", out["session_id"])
	<-srv.StreamOpened()
	require.NoError(t, drv.Send(t.Context(), "interrupt", nil))

	srv.Event("1", "approval", `{"approval_id":"a7"}`)
	ev := nextEvent(t, drv)
	require.Equal(t, "permission", ev.Canonical)
	require.JSONEq(t, `{"request":"answer","id":"a7"}`, string(ev.AskID))
	require.NoError(t, drv.Reply(ev.AskID, []byte(`{"decision":"allow"}`)))

	srv.Event("2", "done", `{}`)
	require.Equal(t, "turn_stop", nextEvent(t, drv).Canonical)
	var bodies []string
	for _, q := range srv.Requests() {
		if q.Method == "POST" {
			bodies = append(bodies, q.Path+" "+q.Body)
		}
	}
	require.Equal(t, []string{
		"/runs ", `/runs/r1/say {"text":"hi"}`, "/runs/r1/cancel ", `/runs/r1/answers/a7 {"decision":"allow"}`,
	}, bodies)
}
