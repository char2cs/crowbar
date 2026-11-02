package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/stream/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type noOperations struct{}

func (noOperations) Snapshot(context.Context) ([]domain.Operation, error) { return nil, nil }

type noSidebar struct{}

func (noSidebar) Entities(context.Context) ([]appstream.Entity, error) { return nil, nil }

func serve(t *testing.T, keep ws.KeepAlive) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	hub := appstream.New("e1", map[stream.ScopeKind]appstream.Source{
		stream.KindProjects: appstream.NewProjectsSource(noOperations{}, noSidebar{}, nil),
	}, nil, nil)
	r := gin.New()
	r.GET("/v0/stream", handlers.New(hub, keep).Stream)
	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		hub.Close()
		srv.Close()
	})
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/v0/stream"
}

func TestStream_ServesReadyThenASubscribedScope(t *testing.T) {
	conn, resp, err := websocket.DefaultDialer.Dial(serve(t, ws.DefaultKeepAlive), nil)
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
	_, resp, err := websocket.DefaultDialer.Dial(serve(t, ws.DefaultKeepAlive), header)
	require.Error(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readEnds reads until the server ends the socket, and returns how.
func readEnds(t *testing.T, conn *websocket.Conn) error {
	t.Helper()
	ended := make(chan error, 1)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				ended <- err
				return
			}
		}
	}()
	select {
	case err := <-ended:
		return err
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the server never ended the socket")
		return nil
	}
}

// A half-open peer is found by its missing pongs, not left holding a queue.
func TestStream_APeerThatNeverPongsIsClosed(t *testing.T) {
	conn := dial(t, serve(t, ws.KeepAlive{Ping: 10 * time.Millisecond, Pong: 100 * time.Millisecond, Write: time.Second}))
	conn.SetPingHandler(func(string) error { return nil })

	require.Error(t, readEnds(t, conn))
}

func TestStream_AnOversizeMessageClosesTheSocket(t *testing.T) {
	conn := dial(t, serve(t, ws.DefaultKeepAlive))
	require.NoError(t, conn.WriteJSON(stream.Request{Op: stream.OpSub, Scope: strings.Repeat("x", 2048)}))

	err := readEnds(t, conn)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseMessageTooBig), "%v", err)
}
