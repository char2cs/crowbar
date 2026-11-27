//go:build integration

package v0_test

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/testutil/accessfake"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/core/logring"
)

func TestConsoleLogs_WithRing_StreamsReplayReadyAndLiveRedacted(t *testing.T) {
	tc := newApp(t)
	ring := logring.New()
	log := slog.New(ring.Wrap(slog.NewTextHandler(&strings.Builder{}, nil)))
	log.Info("old", "component", "boot")
	r := gin.New()
	c := v0.New(tc.app, tc.eng, accessfake.Closed(), v0.WithLogs(ring))
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v0/console/logs", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	var replayed, ready, live map[string]any
	require.NoError(t, conn.ReadJSON(&replayed))
	require.NoError(t, conn.ReadJSON(&ready))
	log.Warn("fresh", "api_token", "s3cret")
	require.NoError(t, conn.ReadJSON(&live))

	assert.Equal(t, "old", replayed["msg"])
	assert.Equal(t, "boot", replayed["component"])
	assert.Equal(t, map[string]any{"type": "ready", "seq": float64(1)}, ready)
	assert.Equal(t, map[string]any{"api_token": "[redacted]"}, live["fields"])
}
