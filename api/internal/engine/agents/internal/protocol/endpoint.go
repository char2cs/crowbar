package protocol

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/connect"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/httpdriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsconn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// ConnectExec runs a descriptor's connect command and returns its stdout.
type ConnectExec = connect.Exec

// Endpoint is what a websocket or http descriptor needs to reach its server: the
// connect command's argv, and the env both it and auth.bearer_env read.
type Endpoint struct {
	Argv []string
	Env  []string
	Run  ConnectExec
}

// Headers a descriptor may never set from connect output: the transport's own.
var forbiddenHeaders = []string{
	"Host", "Cookie", "Authorization", "Proxy-Authorization", "Connection", "Upgrade", "Content-Length", "Origin",
}

// StartAPIEndpoint connects a descriptor whose delivery is websocket or http,
// with no serve process, and returns once its session can be established.
func StartAPIEndpoint(ctx context.Context, d *spec.Descriptor, ep Endpoint, origin APISessionOrigin) (*APIConn, error) {
	var (
		drv *apidriver.Driver
		err error
	)
	switch d.Runtime.API.Delivery {
	case spec.APIDeliveryWebsocket:
		drv, err = startWebsocket(ctx, d, ep, origin)
	case spec.APIDeliveryHTTP:
		drv, err = startHTTP(ctx, d, ep, origin)
	default:
		return nil, fmt.Errorf("protocol: %s: delivery %q is not dialed", d.ID, d.Runtime.API.Delivery)
	}
	if err != nil {
		return nil, err
	}
	return &APIConn{drv: drv}, nil
}

func startWebsocket(ctx context.Context, d *spec.Descriptor, ep Endpoint, origin APISessionOrigin) (*apidriver.Driver, error) {
	api := d.Runtime.API
	rc := apidriver.Reconnect{Max: api.Reconnect.Max, Backoff: time.Duration(api.Reconnect.BackoffMS) * time.Millisecond}
	return apidriver.StartDial(ctx, d, websocketDialer(d, ep), rc, origin)
}

// CallOnce reaches a websocket descriptor, runs its handshake, makes the one named
// call and hangs up. It never creates a session.
func CallOnce(ctx context.Context, d *spec.Descriptor, ep Endpoint, method string, params map[string]any) (json.RawMessage, error) {
	if d.Runtime.API.Delivery != spec.APIDeliveryWebsocket {
		return nil, fmt.Errorf("protocol: %s: delivery %q cannot make a one-off call", d.ID, d.Runtime.API.Delivery)
	}
	return apidriver.CallOnce(ctx, d, websocketDialer(d, ep), method, params)
}

func websocketDialer(d *spec.Descriptor, ep Endpoint) func(context.Context) (jsonrpc.Transport, error) {
	api := d.Runtime.API
	resolve := func(ctx context.Context) (connect.Endpoint, error) {
		if len(api.Connect) == 0 {
			return connect.Endpoint{URL: api.StaticURL, Header: http.Header{}}, nil
		}
		fields := connect.Fields{
			URL: api.ConnectMap.URL, Protocols: api.ConnectMap.Protocols,
			Headers: api.ConnectMap.Headers, Credential: api.ConnectMap.Credential,
		}
		return connect.Resolve(ctx, ep.Run, ep.Argv, ep.Env, fields, allowedHeaders(api.ConnectMap.Headers))
	}
	return func(ctx context.Context) (jsonrpc.Transport, error) {
		c, err := wsconn.Dial(ctx, resolve)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

func allowedHeaders(declared map[string]string) []string {
	var out []string
	for name := range declared {
		if !forbidden(name) {
			out = append(out, name)
		}
	}
	return out
}

func forbidden(name string) bool {
	canon := http.CanonicalHeaderKey(name)
	if strings.HasPrefix(canon, "Sec-Websocket-") {
		return true
	}
	for _, f := range forbiddenHeaders {
		if f == canon {
			return true
		}
	}
	return false
}

func startHTTP(ctx context.Context, d *spec.Descriptor, ep Endpoint, origin APISessionOrigin) (*apidriver.Driver, error) {
	api := d.Runtime.API
	if len(api.ConnectMap.Headers) > 0 || api.ConnectMap.Protocols != "" {
		return nil, fmt.Errorf("protocol: %s: connect_map headers and protocols are for websocket delivery", d.ID)
	}
	base, bearer, err := httpTarget(ctx, d, ep)
	if err != nil {
		return nil, err
	}
	requests := make(map[string]httpdriver.Request, len(api.Requests))
	for name, r := range api.Requests {
		req := httpdriver.Request{Method: r.Method, Path: r.Path, Capture: r.Capture, OKStatus: r.OKStatus}
		if len(r.Body) > 0 {
			req.Body = r.Body
		}
		requests[name] = req
	}
	cfg := httpdriver.Config{
		BaseURL: base, Requests: requests, StreamPath: api.Stream.Path, Bearer: bearer,
		MaxReconnects: api.Reconnect.Max,
	}
	if ms := api.Reconnect.BackoffMS; ms > 0 {
		cfg.Backoff = func(attempt int) time.Duration { return time.Duration(attempt*ms) * time.Millisecond }
	}
	return apidriver.StartHTTP(ctx, d, httpdriver.New(cfg), origin)
}

// httpTarget is the base URL and bearer token of an http descriptor: static or
// printed by connect, the token from auth.bearer_env or connect's own output.
func httpTarget(ctx context.Context, d *spec.Descriptor, ep Endpoint) (base, bearer string, err error) {
	api := d.Runtime.API
	if name := api.Auth.BearerEnv; name != "" {
		var ok bool
		if bearer, ok = env.Lookup(ep.Env, name); !ok || bearer == "" {
			return "", "", fmt.Errorf("protocol: %s: auth.bearer_env %s is not set", d.ID, name)
		}
	}
	if len(api.Connect) == 0 {
		return api.StaticURL, bearer, nil
	}
	fields := connect.Fields{
		URL: api.ConnectMap.URL, Credential: cmp.Or(api.ConnectMap.Credential, api.Auth.BearerPath),
		Schemes: []string{"http", "https"},
	}
	found, err := connect.Resolve(ctx, ep.Run, ep.Argv, ep.Env, fields, nil)
	if err != nil {
		return "", "", err
	}
	if token, ok := strings.CutPrefix(found.Header.Get("Authorization"), "Bearer "); ok {
		bearer = token
	}
	return found.URL, bearer, nil
}
