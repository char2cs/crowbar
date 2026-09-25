//go:build integration

package v0_test

import (
	"context"
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
	"github.com/char2cs/crowbar/api/internal/domain"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
	enginelsp "github.com/char2cs/crowbar/api/internal/engine/lsp"
	"github.com/char2cs/crowbar/api/tests/kit"
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

// wsReadBound bounds every frame wait in this package. A correct run never
// reaches it; a regression then fails in seconds under its own test name
// instead of stalling the whole package until `go test -timeout`.
const wsReadBound = 10 * time.Second

func dialV0(
	t *testing.T,
	srv *httptest.Server,
	path string,
) *kit.WSWatcher {
	t.Helper()
	return kit.Dial(t, websocket.DefaultDialer, "ws"+srv.URL[len("http"):]+path)
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
		workspace.CreateInput{ID: "A", RepoID: "rA", ProjectID: "p1", Branch: "main", WorktreePath: repoA, Provisioning: domain.WorkspaceProvisioned},
		now,
	)
	require.NoError(t, err)
	_, err = tc.app.Repositories.Workspace.Create(
		ctx,
		workspace.CreateInput{ID: "B", RepoID: "rB", ProjectID: "p1", Branch: "main", WorktreePath: repoB, Provisioning: domain.WorkspaceProvisioned},
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

	got := conn.ReadMsg(t, wsReadBound)
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
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Provisioning: domain.WorkspacePlaceholder},
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

	got := conn.ReadMsg(t, wsReadBound)
	assert.Equal(t, "chat-1", got["wsId"])
	diags, _ := got["diagnostics"].([]any)
	require.Len(t, diags, 1)
}
