//go:build integration

package v0_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

func TestAppClose_StopsLiveWatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	seedRepo(t, tc, "r1")
	repoPath := gitRepo(t)

	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: repoPath},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	tc.app.Usecases.Worktree = stubChatWorktreeResolver{
		chatToWs:   map[string]string{"chat-1": "w1"},
		workspaces: tc.app.Repositories.Workspace,
	}

	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// spec §8 step 6 retired files' repo-scoped .../workspaces/:wsId/files/ws
	// mount; the flat chat prefix is the only surface left.
	conn := dialWS(t, srv, "/v0/chats/chat-1/files/ws")
	c.WaitFilesRegistered()
	t.Cleanup(func() { _ = conn.Close() })

	require.NotPanics(t, tc.app.Close) // tears down the live watcher and LSP host
}

func TestAppClose_IdempotentAndEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	v0.New(tc.app, tc.eng)

	require.NotPanics(t, tc.app.Close) // no live resources: no-op
	assert.NotPanics(t, tc.app.Close)  // second call is safe
}
