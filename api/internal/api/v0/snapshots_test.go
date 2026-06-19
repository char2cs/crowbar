//go:build integration

package v0_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	osexec "os/exec"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
	enginelsp "github.com/char2cs/crowbar/api/internal/engine/lsp"
)

type seededLSP struct {
	enginelsp.Engine
	diags map[string][]lspdomain.Diagnostic
}

func (s seededLSP) DiagnosticsSnapshot(
	wsID string,
) []lspdomain.Diagnostic {
	return s.diags[wsID]
}

func serveV0(
	t *testing.T,
	appC *app.Container,
	engC *engine.Container,
) (*v0.Container, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := v0.New(appC, engC)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return c, srv
}

func dialV0(
	t *testing.T,
	srv *httptest.Server,
	path string,
) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readSnapshot(
	t *testing.T,
	conn *websocket.Conn,
) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	return got
}

func initGitRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := osexec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run())
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test User")
	return dir
}

// TestSnapshot_Workspaces_DeliveredOnConnect proves the Workspaces
// snapshot-on-subscribe (03 §1a): a client receives the current workspace row
// immediately on connect (before any live Push), with the persisted hasConflicts
// surfaced. With the agent-run concept removed, working is always false.
func TestSnapshot_Workspaces_DeliveredOnConnect(t *testing.T) {
	tc := newApp(t)
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "feat/x"},
		now,
	)
	require.NoError(t, err)
	_, err = tc.app.Repositories.Workspace.SyncWorkingTreeState(
		ctx,
		workspace.SyncInput{ID: "w1", HasConflicts: true, HasCommits: true},
		now.Add(time.Minute),
	)
	require.NoError(t, err)

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/ws/workspaces?projectId=p1")

	got := readSnapshot(t, conn)
	assert.Equal(t, "w1", got["id"])
	assert.Equal(t, false, got["working"])
	assert.Equal(t, true, got["hasConflicts"])
}

// TestSnapshot_Workspaces_ScopePredicateFilters proves the snapshot is filtered
// by the per-client predicate: a projectId=p2 client sees only its workspace.
func TestSnapshot_Workspaces_ScopePredicateFilters(t *testing.T) {
	tc := newApp(t)
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"},
		now,
	)
	require.NoError(t, err)
	_, err = tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w2", RepoID: "r2", ProjectID: "p2"},
		now,
	)
	require.NoError(t, err)

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/ws/workspaces?projectId=p2")

	got := readSnapshot(t, conn)
	assert.Equal(t, "w2", got["id"])
}

// TestSnapshot_Chats_DeliveredOnConnect proves the Chats snapshot-on-subscribe:
// a wsId-scoped client receives the current ChatStatusEvent on connect.
func TestSnapshot_Chats_DeliveredOnConnect(t *testing.T) {
	tc := newApp(t)
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"},
		now,
	)
	require.NoError(t, err)
	_, err = tc.app.Repositories.Chat.Create(ctx, "c1", "w1", "title", now)
	require.NoError(t, err)
	_, err = tc.app.Repositories.Chat.Create(ctx, "c2", "w2", "other", now)
	require.NoError(t, err)

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/ws/chats?wsId=w1")

	got := readSnapshot(t, conn)
	assert.Equal(t, "c1", got["chatId"])
	assert.Equal(t, "w1", got["wsId"])
	assert.Equal(t, "idle", got["status"])
}

// TestSnapshot_Git_DeliveredOnConnectScoped proves the Git snapshot-on-subscribe:
// a wsId-scoped client receives the current GitStatus of its workspace only.
func TestSnapshot_Git_DeliveredOnConnectScoped(t *testing.T) {
	tc := newApp(t)
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()
	repoA := initGitRepo(t)
	repoB := initGitRepo(t)

	_, err := tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "A", RepoID: "rA", ProjectID: "p1", Branch: "main", WorktreePath: repoA},
		now,
	)
	require.NoError(t, err)
	_, err = tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "B", RepoID: "rB", ProjectID: "p1", Branch: "main", WorktreePath: repoB},
		now,
	)
	require.NoError(t, err)

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/ws/git?wsId=A")

	got := readSnapshot(t, conn)
	assert.Equal(t, "main", got["branch"])
	_, hasWsID := got["wsId"]
	assert.False(t, hasWsID, "git payload is bare GitStatus")
}

// TestSnapshot_LSP_DeliveredOnConnect proves the LSP snapshot-on-subscribe: a
// wsId-scoped client receives the engine's current diagnostics for its workspace.
func TestSnapshot_LSP_DeliveredOnConnect(t *testing.T) {
	tc := newApp(t)
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"},
		now,
	)
	require.NoError(t, err)
	tc.eng.LSP = seededLSP{
		Engine: tc.eng.LSP,
		diags:  map[string][]lspdomain.Diagnostic{"w1": {{Message: "boom"}}},
	}

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/ws/lsp?wsId=w1")

	got := readSnapshot(t, conn)
	assert.Equal(t, "w1", got["wsId"])
	diags, _ := got["diagnostics"].([]any)
	require.Len(t, diags, 1)
}
