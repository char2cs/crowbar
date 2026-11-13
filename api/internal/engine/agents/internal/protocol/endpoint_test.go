package protocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func httpEndpointDescriptor(api spec.APISpec) *spec.Descriptor {
	api.Delivery = spec.APIDeliveryHTTP
	api.Requests = map[string]spec.RequestSpec{
		"create": {Method: "POST", Path: "/runs", Capture: map[string]string{"run_id": "id"}},
	}
	api.Stream = spec.StreamSpec{Path: "/runs/{run_id}/events"}
	return &spec.Descriptor{
		ID:      "web",
		Runtime: spec.RuntimeSpec{API: api},
		Events: map[string]spec.EventSpec{
			"prompt": {Fresh: []spec.CallStep{{Call: "create", Capture: map[string]string{"session_id": "id"}}}},
		},
	}
}

func printing(out string) protocol.ConnectExec {
	return func(context.Context, []string, []string) ([]byte, error) { return []byte(out), nil }
}

func openRun(t *testing.T, d *spec.Descriptor, ep protocol.Endpoint, srv *kit.FakeHTTP) []kit.RecordedRequest {
	t.Helper()
	conn, err := protocol.StartAPIEndpoint(t.Context(), d, ep, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.EstablishSession(t.Context(), "prompt", nil)
	require.NoError(t, err)
	return srv.Requests()
}

func TestStartAPIEndpoint_HTTPStaticURLSendsTheBearerFromTheEnv(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/runs", 200, `{"id":"r1"}`)
	srv.StreamAt("/runs/r1/events")
	d := httpEndpointDescriptor(spec.APISpec{StaticURL: srv.URL, Auth: spec.AuthSpec{BearerEnv: "SVC_TOKEN"}})

	reqs := openRun(t, d, protocol.Endpoint{Env: []string{"SVC_TOKEN=secret"}}, srv)

	require.NotEmpty(t, reqs)
	assert.True(t, reqs[0].HasAuth)
}

func TestStartAPIEndpoint_HTTPRefusesAnUnsetBearerEnv(t *testing.T) {
	d := httpEndpointDescriptor(spec.APISpec{StaticURL: "http://127.0.0.1:1", Auth: spec.AuthSpec{BearerEnv: "SVC_TOKEN"}})

	_, err := protocol.StartAPIEndpoint(t.Context(), d, protocol.Endpoint{Env: []string{"OTHER=x"}}, nil)

	require.ErrorContains(t, err, "auth.bearer_env SVC_TOKEN is not set")
}

func TestStartAPIEndpoint_HTTPTakesURLAndTokenFromTheConnectCommand(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/runs", 200, `{"id":"r1"}`)
	srv.StreamAt("/runs/r1/events")
	d := httpEndpointDescriptor(spec.APISpec{
		Connect:    []string{"dial"},
		ConnectMap: spec.ConnectMap{URL: "endpoint.url"},
		Auth:       spec.AuthSpec{BearerPath: "endpoint.token"},
	})
	ep := protocol.Endpoint{Argv: []string{"dial"}, Run: printing(`{"endpoint":{"url":"` + srv.URL + `","token":"t"}}`)}

	reqs := openRun(t, d, ep, srv)

	require.NotEmpty(t, reqs)
	assert.True(t, reqs[0].HasAuth)
}

func TestStartAPIEndpoint_HTTPRejectsAConnectURLThatIsNotHTTP(t *testing.T) {
	d := httpEndpointDescriptor(spec.APISpec{Connect: []string{"dial"}, ConnectMap: spec.ConnectMap{URL: "url"}})
	ep := protocol.Endpoint{Argv: []string{"dial"}, Run: printing(`{"url":"ws://host/x"}`)}

	_, err := protocol.StartAPIEndpoint(t.Context(), d, ep, nil)

	require.ErrorContains(t, err, "http or https URL")
}

func TestStartAPIEndpoint_HTTPRefusesWebsocketOnlyConnectFields(t *testing.T) {
	d := httpEndpointDescriptor(spec.APISpec{
		StaticURL: "http://127.0.0.1:1", ConnectMap: spec.ConnectMap{Headers: map[string]string{"X-A": "a"}},
	})

	_, err := protocol.StartAPIEndpoint(t.Context(), d, protocol.Endpoint{}, nil)

	require.ErrorContains(t, err, "for websocket delivery")
}

func TestStartAPIEndpoint_RefusesADeliveryItDoesNotDial(t *testing.T) {
	d := &spec.Descriptor{ID: "p", Runtime: spec.RuntimeSpec{API: spec.APISpec{Delivery: spec.APIDeliveryStdio}}}

	_, err := protocol.StartAPIEndpoint(t.Context(), d, protocol.Endpoint{}, nil)

	require.ErrorContains(t, err, "is not dialed")
}

func TestStartAPIEndpoint_WebsocketRefusesHeadersTheTransportOwns(t *testing.T) {
	for _, name := range []string{"Authorization", "Host", "Sec-WebSocket-Key", "origin"} {
		d := &spec.Descriptor{ID: "p", Runtime: spec.RuntimeSpec{API: spec.APISpec{
			Delivery: spec.APIDeliveryWebsocket, Connect: []string{"dial"},
			ConnectMap: spec.ConnectMap{URL: "url", Headers: map[string]string{name: "h"}},
		}}}
		ep := protocol.Endpoint{Argv: []string{"dial"}, Run: printing(`{"url":"ws://127.0.0.1:1","h":"v"}`)}

		_, err := protocol.StartAPIEndpoint(t.Context(), d, ep, nil)

		require.ErrorContains(t, err, "is not allowed", name)
	}
}

func TestStartAPIEndpoint_WebsocketSurfacesAFailingConnectCommand(t *testing.T) {
	d := &spec.Descriptor{ID: "p", Runtime: spec.RuntimeSpec{API: spec.APISpec{
		Delivery: spec.APIDeliveryWebsocket, Connect: []string{"dial"}, ConnectMap: spec.ConnectMap{URL: "url"},
	}}}
	boom := errors.New("boom")
	ep := protocol.Endpoint{Argv: []string{"dial"}, Run: func(context.Context, []string, []string) ([]byte, error) { return nil, boom }}

	_, err := protocol.StartAPIEndpoint(t.Context(), d, ep, nil)

	require.ErrorIs(t, err, boom)
}

func TestCallOnce_RefusesANonWebsocketDelivery(t *testing.T) {
	d := httpEndpointDescriptor(spec.APISpec{})

	_, err := protocol.CallOnce(t.Context(), d, protocol.Endpoint{}, "list", nil)

	require.ErrorContains(t, err, "cannot make a one-off call")
}

func TestCallOnce_DialsTheStaticURLHandshakesAndReturnsTheResult(t *testing.T) {
	ws := kit.NewFakeWS(t)
	d := &spec.Descriptor{ID: "p", Runtime: spec.RuntimeSpec{API: spec.APISpec{
		Delivery: spec.APIDeliveryWebsocket, StaticURL: ws.URL(ws.NewTicket()),
		Handshake: spec.HandshakeSpec{Call: "hello"},
	}}}
	go func() {
		for frame := range ws.Frames {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(frame, &req) != nil || len(req.ID) == 0 {
				continue
			}
			result := `{}`
			if req.Method == "list" {
				result = `{"n":2}`
			}
			if ws.Send([]byte(`{"id":`+string(req.ID)+`,"result":`+result+`}`)) != nil {
				return
			}
		}
	}()

	raw, err := protocol.CallOnce(t.Context(), d, protocol.Endpoint{}, "list", map[string]any{})

	require.NoError(t, err)
	assert.JSONEq(t, `{"n":2}`, string(raw))
}
