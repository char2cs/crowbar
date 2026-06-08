//go:build integration

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

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/api/v0/v0test"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// readUntil loop-reads from conn (under a deadline) until match returns true for
// a decoded message, then returns that message. It avoids time.Sleep races: the
// broadcast may be preceded by other projected rows on the same topic.
func readUntil(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	for {
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(msg, &got))
		if match(got) {
			return got
		}
		require.True(t, time.Now().Before(deadline), "deadline exceeded before match")
	}
}

// TestWave3_WorkspaceCommand_ReachesWSClient proves the FULL Wave 3 chain:
// a real Asynx command on the Workspace aggregate (repo Create / SyncWorkingTreeState,
// both SendWait) -> the read-model projection registered in app.New fires ->
// hub.BroadcastWorkspace -> the Workspaces broadcaster -> a connected WS client.
func TestWave3_WorkspaceCommand_ReachesWSClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	cw := v0test.Wrap(c)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/workspaces?projectId=p1"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	cw.WaitWorkspacesRegistered()

	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	// Command 1: Create. SendWait blocks until the command + read-model projection
	// cycle completes, so the broadcast has already fired when this returns.
	_, err = tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "feat/x"},
		now,
	)
	require.NoError(t, err)

	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == "w1"
	})
	assert.Equal(t, "w1", created["id"])
	assert.Equal(t, "feat/x", created["branch"])
	assert.Equal(t, "p1", created["projectId"])
	// A freshly created workspace carries the "new" status badge.
	assert.Equal(t, "new", created["status"])

	// Command 2: SyncWorkingTreeState — a git working-tree summary mutation. With
	// HasCommits=true the "new" badge clears and the added count is recorded; this
	// proves a git-summary mutation propagates the UPDATED row over WS.
	_, err = tc.app.Repositories.Workspace.SyncWorkingTreeState(
		ctx,
		workspace.SyncInput{ID: "w1", Added: 7, Deleted: 2, HasCommits: true},
		now.Add(time.Minute),
	)
	require.NoError(t, err)

	updated := readUntil(t, conn, func(m map[string]any) bool {
		// The updated row reflects the new added count.
		return m["id"] == "w1" && m["added"] == float64(7)
	})
	assert.Equal(t, "w1", updated["id"])
	assert.Equal(t, float64(7), updated["added"])
	assert.Equal(t, float64(2), updated["deleted"])
	// Status badge "new" cleared (omitempty -> absent) once the branch has commits.
	_, hasStatus := updated["status"]
	assert.False(t, hasStatus, "status badge should clear after HasCommits")
}

// TestWave3_AgentRunProjection_DrivesChatStatusOverWS proves the AgentRun ->
// Chat projection chain: an AgentRun MarkRunning command -> the AgentRun
// projection (registered via RegisterHubProjections in app.New) -> Chat
// SetAgentRunning -> the Chat read-model projection -> hub.BroadcastChat -> the
// Chats broadcaster -> a connected WS client receives status=agent-running.
func TestWave3_AgentRunProjection_DrivesChatStatusOverWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	cw := v0test.Wrap(c)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/chats?wsId=w1"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	cw.WaitChatsRegistered()

	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	// Create the chat (this itself broadcasts an idle ChatStatusEvent).
	_, err = tc.app.Repositories.Chat.Create(ctx, "c1", "w1", "title", now)
	require.NoError(t, err)

	// Drive an AgentRun: Create then MarkRunning. MarkRunning's projection sets the
	// chat to agent-running, whose own projection broadcasts the status event.
	_, err = tc.app.Repositories.AgentRun.Create(ctx, "run1", "w1", "c1", now)
	require.NoError(t, err)
	_, err = tc.app.Repositories.AgentRun.MarkRunning(ctx, "run1")
	require.NoError(t, err)

	// The idle create-event may arrive first; loop until the agent-running event.
	got := readUntil(t, conn, func(m map[string]any) bool {
		return m["chatId"] == "c1" && m["status"] == "agent-running"
	})
	assert.Equal(t, "c1", got["chatId"])
	assert.Equal(t, "w1", got["wsId"])
	assert.Equal(t, "agent-running", got["status"])
}
