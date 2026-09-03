package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// This file pins the handler half of files' move onto /v0/chats/:chatId (spec
// §4.2's shared bucket, §8 step 4): WHICH worktree a handler acts on, for each
// of the two prefixes the surface is currently mounted at.
//
// The trap it exists for is quiet rather than loud. Every files handler read
// ctx.Param("wsId"); on a chat-scoped route that param does not exist, so the
// handler would have called the file usecase with the EMPTY string — not a 404,
// not a panic, just every read and every WRITE aimed at nothing. Asserting a
// status code would not have caught it; only asserting the id the usecase was
// actually handed does. A write is the sharper half of that: a save reaching
// the wrong worktree, or none, loses the user's edit.

// recordingFiles is stubFiles with one question answered: which workspace id
// did the handler pass? It embeds the shared stub so the 8-method Files
// interface stays defined in one place and only the calls under test are
// overridden.
type recordingFiles struct {
	stubFiles
	seen []string
}

func (r *recordingFiles) record(
	wsID string,
) {
	r.seen = append(r.seen, wsID)
}

// last is the workspace id the most recent call was handed. Every files handler
// is synchronous — there is no 202-and-detach path here the way git's writes
// have — so the call has already happened by the time ServeHTTP returns.
func (r *recordingFiles) last(
	t *testing.T,
) string {
	t.Helper()
	require.NotEmpty(t, r.seen, "the handler never reached the file usecase at all")
	return r.seen[len(r.seen)-1]
}

func (r *recordingFiles) Tree(
	_ context.Context,
	wsID string,
	_ string,
	_ file.FileStatusProvider,
) ([]domain.FileNode, error) {
	r.record(wsID)
	return []domain.FileNode{}, nil
}

func (r *recordingFiles) WriteContent(
	_ context.Context,
	wsID string,
	_, _, _ string,
	_ time.Time,
) error {
	r.record(wsID)
	return nil
}

func (r *recordingFiles) Delete(
	_ context.Context,
	wsID string,
	_ string,
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

// filesRouterForScopes wires files' two live mounts the way router.go does,
// including the chat group's middleware — the piece that makes a chat-scoped
// request resolvable at all.
func filesRouterForScopes(
	t *testing.T,
	f handlers.Files,
) *gin.Engine {
	t.Helper()
	h := handlers.New(f)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/files/tree", h.Tree)
	chatScoped.PUT("/files/content", h.SaveContent)
	chatScoped.DELETE("/files", h.Delete)

	wsScoped := r.Group("/v0")
	wsScoped.GET("/workspaces/:wsId/files/tree", h.Tree)
	wsScoped.PUT("/workspaces/:wsId/files/content", h.SaveContent)
	wsScoped.DELETE("/workspaces/:wsId/files", h.Delete)

	return r
}

// TestChatScopedRead_ActsOnTheResolvedWorktreeNotTheChat is the core assertion:
// a read reached through a chat operates on the WORKSPACE that chat resolves
// to. Neither the chat id nor the empty string is an acceptable answer, and
// both are what the obvious wrong implementations produce.
func TestChatScopedRead_ActsOnTheResolvedWorktreeNotTheChat(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	rec := do(r, http.MethodGet, "/v0/chats/chat-1/files/tree", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	got := files.last(t)
	assert.Equal(t, "ws-resolved", got)
	assert.NotEqual(t, "chat-1", got, "the chat id is not a workspace id")
	assert.NotEmpty(t, got, "an unresolved :wsId would silently aim the file usecase at nothing")
}

// TestChatScopedWrite_ActsOnTheResolvedWorktree covers the write path, which
// reads the seam before binding its body and so could have been missed by a
// partial re-key. A save aimed at the empty workspace is the worst outcome in
// this group: the edit goes nowhere and the editor reports success.
func TestChatScopedWrite_ActsOnTheResolvedWorktree(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	rec := do(r, http.MethodPut, "/v0/chats/chat-1/files/content",
		map[string]string{"path": "a.go", "content": "hello"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-resolved", files.last(t))
}

// TestChatScopedDelete_ActsOnTheResolvedWorktree covers the destructive verb,
// whose body parsing has its own query-param fallback and so takes a different
// route through the handler than the other two.
func TestChatScopedDelete_ActsOnTheResolvedWorktree(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	rec := do(r, http.MethodDelete, "/v0/chats/chat-1/files", map[string]string{"path": "a.go"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-resolved", files.last(t))
}

// TestWorkspaceScopedRoutesStillActOnTheirPathParam is the regression bar for
// the mount this step deliberately leaves standing — and for the HOME mount
// with it, which reaches these same handlers through its own group with a
// :wsId that RequireHomeWorkspace injects. The old route keeps naming its
// workspace directly, and reqscope — never set on either group — must not have
// displaced it.
func TestWorkspaceScopedRoutesStillActOnTheirPathParam(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"read", http.MethodGet, "/v0/workspaces/ws-direct/files/tree", nil},
		{
			"write", http.MethodPut, "/v0/workspaces/ws-direct/files/content",
			map[string]string{"path": "a.go", "content": "hello"},
		},
		{
			"delete", http.MethodDelete, "/v0/workspaces/ws-direct/files",
			map[string]string{"path": "a.go"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := &recordingFiles{}
			r := filesRouterForScopes(t, files)

			rec := do(r, tc.method, tc.path, tc.body)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "ws-direct", files.last(t))
		})
	}
}

// TestBothMountsReachTheSameHandlerSet proves the two prefixes are one surface
// rather than two implementations kept in step by hand: the same handler value
// answers both, so a behaviour change lands on both at once.
func TestBothMountsReachTheSameHandlerSet(t *testing.T) {
	files := &recordingFiles{}
	r := filesRouterForScopes(t, files)

	require.Equal(t, http.StatusOK,
		do(r, http.MethodGet, "/v0/chats/chat-1/files/tree", nil).Code)
	viaChat := files.last(t)

	require.Equal(t, http.StatusOK,
		do(r, http.MethodGet, "/v0/workspaces/ws-resolved/files/tree", nil).Code)
	viaWorkspace := files.last(t)

	assert.Equal(t, viaWorkspace, viaChat,
		"one worktree named two ways must reach the file usecase identically")
}
