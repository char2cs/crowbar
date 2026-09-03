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

// This file pins the handler half of git's move onto /v0/chats/:chatId (spec
// §4.2's shared bucket, §8 step 4): WHICH worktree a handler acts on, for each
// of the two prefixes the surface is currently mounted at.
//
// The trap it exists for is quiet rather than loud. Every git handler read
// ctx.Param("wsId"); on a chat-scoped route that param does not exist, so the
// handler would have called the git usecase with the EMPTY string — not a 404,
// not a panic, just every git operation aimed at nothing. Asserting a status
// code would not have caught it; only asserting the id the usecase was actually
// handed does.

// recordingGit is stubGit with one question answered: which workspace id did
// the handler pass? It embeds the shared stub so the 32-method Git interface
// stays defined in one place and only the calls under test are overridden.
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

// awaitCall blocks until one more call has been recorded. The async write
// handlers answer 202 and do the git work on a detached goroutine, so the
// recorded id has to be waited FOR — on the call's own signal, never a sleep.
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

// resolvedWorkspace is the workspace the chat group's resolveChatWorktree
// middleware stashes for chat "chat-1". Its id deliberately differs from every
// chat id below: a handler that reached for the chat id instead of resolving it
// would pass this test's siblings by luck otherwise.
func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", ProjectID: "p1", RepoID: "r1"}
}

// gitRouterForScopes wires git's two live mounts the way router.go does,
// including the chat group's middleware — the piece that makes a chat-scoped
// request resolvable at all.
func gitRouterForScopes(
	t *testing.T,
	git handlers.Git,
) *gin.Engine {
	t.Helper()
	h := handlers.New(git, &fakeLastErrors{}, noopWork{})
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/git/status", h.Status)
	chatScoped.POST("/git/commit", h.Commit)
	chatScoped.POST("/git/push", h.Push)

	wsScoped := r.Group("/v0")
	wsScoped.GET("/workspaces/:wsId/git/status", h.Status)
	wsScoped.POST("/workspaces/:wsId/git/commit", h.Commit)
	wsScoped.POST("/workspaces/:wsId/git/push", h.Push)

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

// TestChatScopedRead_ActsOnTheResolvedWorktreeNotTheChat is the core assertion:
// a read reached through a chat operates on the WORKSPACE that chat resolves
// to. Neither the chat id nor the empty string is an acceptable answer, and
// both are what the obvious wrong implementations produce.
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

// TestChatScopedWrite_ActsOnTheResolvedWorktree covers the synchronous write
// path, which reads the same seam through a different local name (wsID rather
// than id) and so could have been missed by a partial re-key.
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

// TestChatScopedAsyncWrite_ActsOnTheResolvedWorktree covers the 202 + detached
// goroutine path (runAsync), where the workspace id is captured into a closure
// that outlives the request. It is the shape most likely to have kept a stale
// read of the URL param.
func TestChatScopedAsyncWrite_ActsOnTheResolvedWorktree(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	rec := doGitRequest(t, r, http.MethodPost, "/v0/chats/chat-1/git/push", "")

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "ws-resolved", git.awaitCall(t))
}

// TestWorkspaceScopedRoutesStillActOnTheirPathParam is the regression bar for
// the mount this step deliberately leaves standing: the old route keeps naming
// its workspace directly, and reqscope — never set on that group — must not
// have displaced it.
func TestWorkspaceScopedRoutesStillActOnTheirPathParam(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"read", http.MethodGet, "/v0/workspaces/ws-direct/git/status", "", http.StatusOK},
		{"write", http.MethodPost, "/v0/workspaces/ws-direct/git/commit", `{"subject":"s"}`, http.StatusOK},
		{"async write", http.MethodPost, "/v0/workspaces/ws-direct/git/push", "", http.StatusAccepted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doGitRequest(t, r, tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code)
			assert.Equal(t, "ws-direct", git.awaitCall(t))
		})
	}
}

// TestBothMountsReachTheSameHandlerSet proves the two prefixes are one surface
// rather than two implementations kept in step by hand: the same handler value
// answers both, so a behaviour change lands on both at once.
func TestBothMountsReachTheSameHandlerSet(t *testing.T) {
	git := newRecordingGit()
	r := gitRouterForScopes(t, git)

	require.Equal(t, http.StatusOK,
		doGitRequest(t, r, http.MethodGet, "/v0/chats/chat-1/git/status", "").Code)
	viaChat := git.awaitCall(t)

	require.Equal(t, http.StatusOK,
		doGitRequest(t, r, http.MethodGet, "/v0/workspaces/ws-resolved/git/status", "").Code)
	viaWorkspace := git.awaitCall(t)

	assert.Equal(t, viaWorkspace, viaChat,
		"one worktree named two ways must reach the git usecase identically")
}
