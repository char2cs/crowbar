package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/stream/handlers"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type noOperations struct{}

func (noOperations) Snapshot(context.Context) ([]domain.Operation, error) { return nil, nil }

func serve(t *testing.T) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	hub := appstream.New("e1", map[stream.ScopeKind]appstream.Source{
		stream.KindProjects: appstream.NewProjectsSource(noOperations{}, nil),
	}, nil, nil)
	r := gin.New()
	r.GET("/v0/stream", handlers.New(hub).Stream)
	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		hub.Close()
		srv.Close()
	})
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/v0/stream"
}

func TestStream_ServesReadyThenASubscribedScope(t *testing.T) {
	conn, resp, err := websocket.DefaultDialer.Dial(serve(t), nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { require.NoError(t, conn.Close()) })

	var ready stream.Frame
	require.NoError(t, conn.ReadJSON(&ready))
	assert.Equal(t, stream.Ready("e1"), ready)

	require.NoError(t, conn.WriteJSON(stream.Request{Op: stream.OpSub, Scope: "projects"}))
	var synced stream.Frame
	require.NoError(t, conn.ReadJSON(&synced))
	assert.Equal(t, stream.Synced("projects"), synced)
}

func TestStream_RefusesACrossOriginUpgrade(t *testing.T) {
	header := http.Header{"Origin": {"https://evil.example"}}
	_, resp, err := websocket.DefaultDialer.Dial(serve(t), header)
	require.Error(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
