package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestAPITransport_AcceptsAServeOverStdioInNDJSON(t *testing.T) {
	require.NoError(t, rules.Apply(withAPITransport(valid())))
}

// A serve Crowbar cannot speak to is a load failure, not a spawn that waits
// on a channel nobody opened.
func TestAPITransport_RejectsAServeThatDoesNotSayHowItIsSpokenTo(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*spec.APISpec)
		wantMsg string
	}{
		"no delivery":       {func(a *spec.APISpec) { a.Delivery = "" }, "runtime.api.delivery"},
		"a socket delivery": {func(a *spec.APISpec) { a.Delivery = "unix" }, "runtime.api.delivery"},
		"no format":         {func(a *spec.APISpec) { a.Format = "" }, "runtime.api.format"},
		"a framed format":   {func(a *spec.APISpec) { a.Format = "content-length" }, "runtime.api.format"},
	} {
		t.Run(name, func(t *testing.T) {
			d := withAPITransport(valid())
			tc.mutate(&d.Runtime.API)

			err := rules.Apply(d)

			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestAPITransport_ADescriptorWithNoServeDeclaresNone(t *testing.T) {
	d := valid()
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}

	require.NoError(t, rules.Apply(d))
}

func withWebsocket(d *spec.Descriptor) *spec.Descriptor {
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{
		Protocol: "jsonrpc", Delivery: spec.APIDeliveryWebsocket, Format: spec.APIFormatJSONRPCText,
		Connect:    []string{"agent-cli", "ws-ticket", "--json"},
		ConnectMap: spec.ConnectMap{URL: "url", Protocols: "protocols"},
		Handshake:  spec.HandshakeSpec{Call: "hello"},
	}
	return d
}

func withHTTP(d *spec.Descriptor) *spec.Descriptor {
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{
		Delivery: spec.APIDeliveryHTTP, Format: spec.APIFormatSSE,
		Connect:    []string{"agent-cli", "endpoint"},
		ConnectMap: spec.ConnectMap{URL: "base", Credential: "token"},
		Auth:       spec.AuthSpec{BearerPath: "credential"},
		Requests: map[string]spec.RequestSpec{
			"create": {Method: "POST", Path: "/runs", Body: map[string]any{"input": "{prompt}"}, Capture: map[string]string{"run_id": "id"}},
			"stop":   {Method: "POST", Path: "/runs/{run_id}/stop"},
		},
		Stream: spec.StreamSpec{Path: "/runs/{run_id}/events"},
	}
	return d
}

func TestAPITransport_AcceptsWebsocketAndHTTP(t *testing.T) {
	require.NoError(t, rules.Apply(withWebsocket(valid())))
	require.NoError(t, rules.Apply(withHTTP(valid())))

	d := withWebsocket(valid())
	d.Runtime.API.Connect, d.Runtime.API.ConnectMap, d.Runtime.API.StaticURL = nil, spec.ConnectMap{}, "wss://host.example/rpc"
	require.NoError(t, rules.Apply(d))
}

func TestAPITransport_RejectsAnInvalidEndpointDelivery(t *testing.T) {
	for name, tc := range map[string]struct {
		build   func() *spec.Descriptor
		mutate  func(*spec.APISpec)
		wantMsg string
	}{
		"websocket with serve":   {func() *spec.Descriptor { return withWebsocket(valid()) }, func(a *spec.APISpec) { a.Serve = []string{"x"} }, "serve"},
		"websocket wrong format": {func() *spec.Descriptor { return withWebsocket(valid()) }, func(a *spec.APISpec) { a.Format = "ndjson" }, "runtime.api.format"},
		"websocket no endpoint": {func() *spec.Descriptor { return withWebsocket(valid()) }, func(a *spec.APISpec) {
			a.Connect, a.ConnectMap = nil, spec.ConnectMap{}
		}, "connect or static_url"},
		"websocket no url path": {func() *spec.Descriptor { return withWebsocket(valid()) }, func(a *spec.APISpec) { a.ConnectMap.URL = "" }, "connect_map.url"},
		"bad header name":       {func() *spec.Descriptor { return withWebsocket(valid()) }, func(a *spec.APISpec) { a.ConnectMap.Headers = map[string]string{"bad name": "p"} }, "headers"},
		"websocket with auth":   {func() *spec.Descriptor { return withWebsocket(valid()) }, func(a *spec.APISpec) { a.Auth.BearerEnv = "TOKEN" }, "only for delivery http"},
		"http with serve":       {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Serve = []string{"x"} }, "serve"},
		"http wrong format":     {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Format = "ndjson" }, "runtime.api.format"},
		"http no requests":      {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Requests = nil }, "requests"},
		"http no stream":        {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Stream.Path = "" }, "stream"},
		"literal bearer":        {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Auth = spec.AuthSpec{BearerEnv: "sk-live-abc123"} }, "bearer_env"},
		"literal bearer path":   {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Auth = spec.AuthSpec{BearerPath: "Bearer abc 123"} }, "bearer_path"},
		"both bearers":          {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Auth.BearerEnv = "TOKEN" }, "pick one"},
		"unresolvable body":     {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Requests["create"].Body["x"] = "{nowhere}" }, "{nowhere}"},
		"unresolvable stream":   {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Stream.Path = "/{nowhere}" }, "{nowhere}"},
		"bad method":            {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Requests["stop"] = spec.RequestSpec{Method: "FETCH", Path: "/s"} }, "method"},
		"relative path":         {func() *spec.Descriptor { return withHTTP(valid()) }, func(a *spec.APISpec) { a.Requests["stop"] = spec.RequestSpec{Method: "POST", Path: "s"} }, "path"},
	} {
		t.Run(name, func(t *testing.T) {
			d := tc.build()
			tc.mutate(&d.Runtime.API)

			err := rules.Apply(d)

			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestAPITransport_EventRequestReferencesMustExist(t *testing.T) {
	d := withHTTP(valid())
	d.Events["permission"] = spec.EventSpec{Ask: spec.WireRef{"approval"}, ReplyRequest: "answer"}
	err := rules.Apply(d)
	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "reply_request")

	d.Runtime.API.Requests["answer"] = spec.RequestSpec{Method: "POST", Path: "/runs/{run_id}/answer"}
	require.NoError(t, rules.Apply(d))

	d.Events["user_prompt"] = spec.EventSpec{Out: spec.WireRef{"send"}, Transport: "api", Fresh: []spec.CallStep{{Call: "missing"}}}
	err = rules.Apply(d)
	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "missing")
}
