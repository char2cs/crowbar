package httpdriver_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/httpdriver"
	"github.com/char2cs/crowbar/api/tests/kit"
)

const deadline = 5 * time.Second

func newStreaming(t *testing.T, mutate func(*httpdriver.Config)) (*httpdriver.Client, *kit.FakeHTTP) {
	t.Helper()
	srv := kit.NewFakeHTTP(t)
	srv.StreamAt("/events")
	srv.Route("POST", "/start", 200, `{}`)
	cfg := httpdriver.Config{
		BaseURL:    srv.URL,
		Requests:   map[string]httpdriver.Request{"start": {Method: "POST", Path: "/start"}},
		StreamPath: "/events",
		Backoff:    func(int) time.Duration { return 0 },
	}
	if mutate != nil {
		mutate(&cfg)
	}
	c := httpdriver.New(cfg)
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.Call(t.Context(), "start", nil)
	require.NoError(t, err)
	select {
	case <-srv.StreamOpened():
	case <-time.After(deadline):
		t.Fatal("stream never opened")
	}
	return c, srv
}

func next(t *testing.T, c *httpdriver.Client) httpdriver.Frame {
	t.Helper()
	select {
	case f, ok := <-c.Frames():
		require.True(t, ok, "frames closed")
		return f
	case <-time.After(deadline):
		t.Fatal("no frame")
		return httpdriver.Frame{}
	}
}

func TestStream_ParsesEventsCommentsAndMultilineData(t *testing.T) {
	c, srv := newStreaming(t, nil)
	srv.Emit(": keepalive\nretry: 50\n\n")
	srv.Emit("event: delta\ndata: {\"a\":\ndata: 1}\n\n")
	srv.Emit("data: {\"b\":2}\r\n\r\n")

	f := next(t, c)
	require.Equal(t, "delta", f.Method)
	require.JSONEq(t, `{"a":
1}`, string(f.Params))
	f = next(t, c)
	require.Equal(t, "message", f.Method)
	require.JSONEq(t, `{"b":2}`, string(f.Params))
}

func TestStream_NonJSONDataIsAnErrorFrameNotDropped(t *testing.T) {
	c, srv := newStreaming(t, nil)
	srv.Event("1", "oops", "not json")
	srv.Event("2", "ok", `{}`)

	f := next(t, c)
	require.Equal(t, "oops", f.Method)
	require.Error(t, f.Err)
	require.Equal(t, "ok", next(t, c).Method)
}

func TestStream_ReconnectsWithLastEventID(t *testing.T) {
	c, srv := newStreaming(t, nil)
	srv.Event("7", "one", `{}`)
	require.Equal(t, "one", next(t, c).Method)

	srv.DropStream()
	<-srv.StreamOpened()
	srv.Event("8", "two", `{}`)
	require.Equal(t, "two", next(t, c).Method)

	var ids []string
	for _, r := range srv.Requests() {
		if r.Path == "/events" {
			ids = append(ids, r.LastEventID)
		}
	}
	require.Equal(t, []string{"", "7"}, ids)
}

func TestStream_GivesUpAfterBoundedReconnects(t *testing.T) {
	c, srv := newStreaming(t, func(cfg *httpdriver.Config) { cfg.MaxReconnects = 2 })
	srv.StreamAt("/gone")

	srv.DropStream()
	f := next(t, c)
	require.Error(t, f.Err)
	select {
	case _, ok := <-c.Frames():
		require.False(t, ok)
	case <-time.After(deadline):
		t.Fatal("frames never closed")
	}
}

func TestStream_FrameParamsStayValidJSON(t *testing.T) {
	c, srv := newStreaming(t, nil)
	srv.Event("1", "e", `{"k":"v"}`)
	require.True(t, json.Valid(next(t, c).Params))
}
