package v0_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func newApp(t *testing.T) *app.Container {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a
}

func workspaceFixture() domain.Workspace {
	return domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1"}
}

func TestV0_HubBroadcastReachesWSClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newApp(t)
	c := v0.New(a)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/workspaces"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	a.Hub.BroadcastWorkspace(workspaceFixture())

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}

func TestV0_PushChat_ReachesWSClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newApp(t)
	c := v0.New(a)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/chats"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitChatsRegistered()

	a.Hub.BroadcastChat(hub.ChatStatusEvent{
		ChatID: "c1",
		WsID:   "w1",
		Status: domain.ChatStatusIdle,
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "c1", got["chatId"])
}

func TestV0_WorkspacesFilter_ProjectId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newApp(t)
	c := v0.New(a)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/workspaces?projectId=p1"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	// This workspace has projectId=p1 so it should pass the filter.
	a.Hub.BroadcastWorkspace(workspaceFixture())

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}

func TestV0_WorkspacesFilter_RepoId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newApp(t)
	c := v0.New(a)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/workspaces?repoId=r1"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	a.Hub.BroadcastWorkspace(workspaceFixture())

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}

func TestV0_ChatsFilter_WsId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newApp(t)
	c := v0.New(a)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/chats?wsId=w1"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitChatsRegistered()

	a.Hub.BroadcastChat(hub.ChatStatusEvent{
		ChatID: "c1",
		WsID:   "w1",
		Status: domain.ChatStatusIdle,
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "c1", got["chatId"])
}
