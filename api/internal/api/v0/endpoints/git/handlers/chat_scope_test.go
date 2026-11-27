package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type recordingGit struct {
	stubGit
	mu    sync.Mutex
	seen  []string
	ready chan struct{}
}

func newRecordingGit() *recordingGit {
	return &recordingGit{ready: make(chan struct{}, 8)}
}

func (r *recordingGit) record(
	wsID string,
) {
	r.mu.Lock()
	r.seen = append(r.seen, wsID)
	r.mu.Unlock()
	r.ready <- struct{}{}
}

func (r *recordingGit) awaitCall(
	t *testing.T,
) string {
	t.Helper()
	<-r.ready
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.seen)
	return r.seen[len(r.seen)-1]
}

func (r *recordingGit) Status(
	_ context.Context,
	wsID string,
) (gitdomain.GitStatus, error) {
	r.record(wsID)
	return gitdomain.GitStatus{Branch: "main"}, nil
}

func (r *recordingGit) Commit(
	_ context.Context,
	wsID, _, _ string,
	_ time.Time,
) error {
	r.record(wsID)
	return nil
}

func (r *recordingGit) Push(
	_ context.Context,
	wsID string,
	_ time.Time,
) error {
	r.record(wsID)
	return nil
}

func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", ProjectID: "p1", RepoID: "r1"}
}

func gitRouterForScopes(
	t *testing.T,
	git handlers.Git,
) *gin.Engine {
	t.Helper()
	h := handlers.New(git, newOps(func(domain.Operation) {}))
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/git/status", h.Status)
	chatScoped.POST("/git/commit", h.Commit)
	chatScoped.POST("/git/push", h.Push)

	return r
}

func doGitRequest(
	t *testing.T,
	r *gin.Engine,
	method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, http.NoBody)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestChatScopedRead_ActsOnTheResolvedWorktreeNotTheChat(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	rec := doGitRequest(t, r, http.MethodGet, "/v0/chats/chat-1/git/status", "")

	require.Equal(t, http.StatusOK, rec.Code)
	got := git.awaitCall(t)
	assert.Equal(t, "ws-resolved", got)
	assert.NotEqual(t, "chat-1", got, "the chat id is not a workspace id")
	assert.NotEmpty(t, got, "an unresolved :wsId would silently aim git at nothing")
}

func TestChatScopedWrite_ActsOnTheResolvedWorktree(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	rec := doGitRequest(
		t, r,
		http.MethodPost, "/v0/chats/chat-1/git/commit",
		`{"subject":"a subject"}`,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-resolved", git.awaitCall(t))
}

func TestChatScopedAsyncWrite_ActsOnTheResolvedWorktree(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	rec := doGitRequest(t, r, http.MethodPost, "/v0/chats/chat-1/git/push", "")

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "ws-resolved", git.awaitCall(t))
}

func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	rec := doGitRequest(t, r, http.MethodGet, "/v0/workspaces/ws-direct/git/status", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
