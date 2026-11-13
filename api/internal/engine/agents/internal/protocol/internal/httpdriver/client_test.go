package httpdriver_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/httpdriver"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestReply_SendsTheAskIDAndPayloadThroughTheNamedRequest(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/runs/r1/approval", 200, `{}`)
	c := httpdriver.New(httpdriver.Config{
		BaseURL: srv.URL,
		Requests: map[string]httpdriver.Request{"approve": {
			Method: "POST", Path: "/runs/r1/approval",
			Body: map[string]any{"approval_id": "{ask_id}", "choice": "{choice}"},
		}},
	})
	t.Cleanup(func() { _ = c.Close() })

	require.NoError(t, c.Reply(t.Context(), "approve", "a9", json.RawMessage(`{"choice":"once"}`)))
	require.JSONEq(t, `{"approval_id":"a9","choice":"once"}`, srv.Requests()[0].Body)
}

func TestClose_CancelsStreamAndClosesFrames(t *testing.T) {
	c, _ := newStreaming(t, nil)
	require.NoError(t, c.Close())
	select {
	case _, ok := <-c.Frames():
		require.False(t, ok)
	case <-time.After(deadline):
		t.Fatal("frames never closed")
	}
	_, err := c.Call(t.Context(), "start", nil)
	require.Error(t, err)
	require.NoError(t, c.Close())
}

func TestClose_WithoutAStreamStillClosesFrames(t *testing.T) {
	c := httpdriver.New(httpdriver.Config{BaseURL: "http://127.0.0.1:1"})
	require.NoError(t, c.Close())
	_, ok := <-c.Frames()
	require.False(t, ok)
}
