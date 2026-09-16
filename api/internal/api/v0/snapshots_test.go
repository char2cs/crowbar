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

// readSnapshot blocks until the snapshot frame arrives, then decodes it.
//
// No read deadline: the frame's arrival IS the signal. A snapshot that never
// lands hangs until `go test -timeout` fires and dumps the goroutines, naming
// this test — a real failure, rather than a two-second guess that goes red
// whenever the machine is busy.
func readSnapshot(
	t *testing.T,
	conn *websocket.Conn,
) map[string]any {
	t.Helper()
	// A FAILURE BOUND, not a wait: every caller quiesces the projections first,
	// so the frame is already sitting there and this deadline is never reached
	// on a correct run — it does not slow the suite down and it is not a poll.
	// It exists because the alternative is unbounded. Without it a snapshot that
	// never arrives blocks ReadMessage forever, and Go kills the entire PACKAGE
	// on the 4m timeout: every other test in it is reported as failed, the only
	// clue is a goroutine dump, and the actual culprit is one line of
	// "running tests:" buried in it. That is precisely how this presented in CI.
	// Bounded, the same bug names itself in 10s.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "no snapshot frame within 10s — the projection the "+
		"snapshot is built from had not settled, or nothing matched the scope predicate")
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

// TestSnapshot_Git_DeliveredOnConnectScoped proves the Git snapshot-on-subscribe
// over the flat chat-scoped mount (spec §8 step 6 retired the old
// .../workspaces/:wsId/git/status twin): a chat-scoped client receives the
// current GitStatus of the worktree it resolves to only — gitSnapshot's BARE
// chat-id branch (chatGitSnapshot), exercised end to end through a real
// resolveChatWorktree resolve rather than pinned as a unit test in isolation.
func TestSnapshot_Git_DeliveredOnConnectScoped(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "rA")
	seedRepo(t, tc, "rB")
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
	// wsToChats matters here, not just chatToWs: the replay frame carries the
	// SAME fan-out set a live push would (appendGitStatus), and gitDef's
	// chatId filter is a Required membership match — an empty set would drop
	// the snapshot exactly as it would drop a live frame.
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{
		chatToWs:   map[string]string{"chat-a": "A"},
		wsToChats:  map[string][]string{"A": {"chat-a"}},
		workspaces: tc.app.Repositories.Workspace,
	}

	// Settle the projections before subscribing (see
	// TestSnapshot_Workspaces_DeliveredOnConnect): resolveChatWorktree's own
	// resolve reads the same read model the projection settles.
	tc.app.Repositories.WaitQuiescent()

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/chats/chat-a/git/status")

	got := readSnapshot(t, conn)
	assert.Equal(t, "main", got["branch"])
	_, hasWsID := got["wsId"]
	assert.False(t, hasWsID, "git payload is bare GitStatus")
}

// TestSnapshot_LSP_DeliveredOnConnect proves the LSP snapshot-on-subscribe over
// the flat chat-scoped mount (spec §8 step 6 retired editor/LSP's old
// .../workspaces/:wsId/lsp/ws twin entirely): a chat-scoped client receives
// the engine's current diagnostics for its OWN session — lspSnapshot's BARE
// chat-id branch (chatLSPSnapshot) keys the engine lookup by the chat id
// itself (spec §4.2's OWNED bucket), not by the workspace it resolves to.
func TestSnapshot_LSP_DeliveredOnConnect(t *testing.T) {
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	ctx := context.Background()
	now := time.Unix(1, 0).UTC()

	_, err := tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"},
		now,
	)
	require.NoError(t, err)
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{
		chatToWs:   map[string]string{"chat-1": "w1"},
		workspaces: tc.app.Repositories.Workspace,
	}
	tc.eng.LSP = seededLSP{
		Engine: tc.eng.LSP,
		diags:  map[string][]lspdomain.Diagnostic{"chat-1": {{Message: "boom"}}},
	}

	// Settle the projections before subscribing (see
	// TestSnapshot_Workspaces_DeliveredOnConnect): resolveChatWorktree's own
	// resolve (confirming the chat has a worktree at all) reads the same read
	// model the projection settles.
	tc.app.Repositories.WaitQuiescent()

	_, srv := serveV0(t, tc.app, tc.eng)
	conn := dialV0(t, srv, "/v0/chats/chat-1/lsp/ws")

	got := readSnapshot(t, conn)
	assert.Equal(t, "chat-1", got["wsId"])
	diags, _ := got["diagnostics"].([]any)
	require.Len(t, diags, 1)
}
