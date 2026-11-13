package httpdriver_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/httpdriver"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestCall_CapturesRunIDThenOpensTheStreamOnItsPath(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/runs", 200, `{"data":{"id":"r 1"}}`)
	srv.StreamAt("/runs/r 1/events")
	c := httpdriver.New(httpdriver.Config{
		BaseURL: srv.URL,
		Requests: map[string]httpdriver.Request{"create": {
			Method: "POST", Path: "/runs",
			Body:    map[string]any{"prompt": "{text}", "n": 1},
			Capture: map[string]string{"run_id": "data.id"},
		}},
		StreamPath: "/runs/{run_id}/events",
		Bearer:     "s3cret",
	})
	t.Cleanup(func() { _ = c.Close() })

	out, err := c.Call(t.Context(), "create", json.RawMessage(`{"text":"hi"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"id":"r 1"}}`, string(out))
	select {
	case <-srv.StreamOpened():
	case <-time.After(deadline):
		t.Fatal("stream never opened")
	}
	reqs := srv.Requests()
	require.JSONEq(t, `{"prompt":"hi","n":1}`, reqs[0].Body)
	require.True(t, reqs[0].HasAuth)
}

func TestCall_UnresolvedPathVarIsAnError(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	c := httpdriver.New(httpdriver.Config{BaseURL: srv.URL, Requests: map[string]httpdriver.Request{
		"stop": {Method: "POST", Path: "/runs/{run_id}/stop"},
	}})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(t.Context(), "stop", nil)
	require.ErrorContains(t, err, "run_id")
	require.Empty(t, srv.Requests())
}

func TestCall_ErrorNamesRequestAndStatusButNotBodyOrSecret(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/x", 500, "leaky-body")
	c := httpdriver.New(httpdriver.Config{BaseURL: srv.URL, Bearer: "s3cret", Requests: map[string]httpdriver.Request{
		"x": {Method: "POST", Path: "/x"},
	}})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(t.Context(), "x", nil)
	require.ErrorContains(t, err, "x")
	require.ErrorContains(t, err, "500")
	require.NotContains(t, err.Error(), "leaky-body")
	require.NotContains(t, err.Error(), "s3cret")
}

func TestCall_OKStatusOverridesTheDefault(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/x", 409, `{}`)
	c := httpdriver.New(httpdriver.Config{BaseURL: srv.URL, Requests: map[string]httpdriver.Request{
		"x": {Method: "POST", Path: "/x", OKStatus: []int{409}},
	}})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(t.Context(), "x", nil)
	require.NoError(t, err)
}

func TestCall_CancelledContextFailsTheCall(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/x", 200, `{}`)
	c := httpdriver.New(httpdriver.Config{BaseURL: srv.URL, Requests: map[string]httpdriver.Request{
		"x": {Method: "POST", Path: "/x"},
	}})
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := c.Call(ctx, "x", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCall_UnknownRequestIsAnError(t *testing.T) {
	c := httpdriver.New(httpdriver.Config{BaseURL: "http://127.0.0.1:1"})
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(t.Context(), "nope", nil)
	require.ErrorContains(t, err, "nope")
}
