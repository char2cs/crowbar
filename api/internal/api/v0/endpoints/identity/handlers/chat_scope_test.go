package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// This file pins the handler half of identity's move onto /v0/chats/:chatId
// (spec §4.2's shared bucket, §8 step 4c): WHICH workspace's worktree the
// resolver is invoked with.
//
// The old /v0/workspaces/:wsId/identity mount (and its WorkspaceReader seam)
// is gone as of spec §8 step 6 — resolveWorkspace now reads reqscope alone.

// recordingResolver records the worktree path it was invoked with.
type recordingResolver struct {
	seenPaths []string
}

func (r *recordingResolver) CurrentIdentity(
	_ context.Context,
	worktreePath string,
) gitdomain.Identity {
	r.seenPaths = append(r.seenPaths, worktreePath)
	return gitdomain.Identity{Login: "octocat"}
}

func identityRouterForScopes(
	t *testing.T,
	resolver handlers.IdentityResolver,
	resolved domain.Workspace,
) *gin.Engine {
	t.Helper()
	h := handlers.New(resolver)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolved)
		c.Next()
	})
	chatScoped.GET("/identity", h.Get)

	return r
}

// TestChatScoped_ResolvesFromReqscope is the core assertion: a chat-scoped
// request acts on the WORKSPACE resolveChatWorktree resolved, and the
// resolver sees its worktree path rather than an empty best-effort fallback.
func TestChatScoped_ResolvesFromReqscope(t *testing.T) {
	resolver := &recordingResolver{}
	resolved := domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"}
	r := identityRouterForScopes(t, resolver, resolved)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat-1/identity", http.NoBody)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, resolver.seenPaths)
	assert.Equal(t, "/resolved", resolver.seenPaths[len(resolver.seenPaths)-1])
}

// TestWorkspaceScopedRouteIsGone proves spec §8 step 6's deletion is real: the
// old /v0/workspaces/:wsId/identity mount this handler set used to also serve
// answers nothing on this router any more.
func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	resolver := &recordingResolver{}
	r := identityRouterForScopes(t, resolver, domain.Workspace{ID: "ws-resolved"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-direct/identity", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
