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
// resolver is invoked with, for each of the two prefixes the surface is
// currently mounted at.

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

// failingReader always errors. It stands in for wsReader on the chat-scoped
// mount to prove resolveWorkspace never falls through to it once reqscope has
// already answered.
type failingReader struct{}

func (failingReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, assert.AnError
}

type directReader struct {
	ws domain.Workspace
}

func (d directReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return d.ws, nil
}

func identityRouterForScopes(
	t *testing.T,
	resolver handlers.IdentityResolver,
	wsReader handlers.WorkspaceReader,
	resolved domain.Workspace,
) *gin.Engine {
	t.Helper()
	h := handlers.New(resolver, wsReader)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolved)
		c.Next()
	})
	chatScoped.GET("/identity", h.Get)

	wsScoped := r.Group("/v0")
	wsScoped.GET("/workspaces/:wsId/identity", h.Get)

	return r
}

// TestChatScoped_ResolvesFromReqscopeNeverTheReader is the core assertion: a
// chat-scoped request never falls through to the (here, always-failing)
// wsReader — resolveWorkspace must have taken the reqscope branch, and the
// resolver must see the resolved worktree path rather than an empty best-
// effort fallback.
func TestChatScoped_ResolvesFromReqscopeNeverTheReader(t *testing.T) {
	resolver := &recordingResolver{}
	resolved := domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"}
	r := identityRouterForScopes(t, resolver, failingReader{}, resolved)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat-1/identity", http.NoBody)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, resolver.seenPaths)
	assert.Equal(t, "/resolved", resolver.seenPaths[len(resolver.seenPaths)-1])
}

// TestWorkspaceScopedRouteStillUsesTheReader is the regression bar for the
// mount this step deliberately leaves standing: the old route keeps resolving
// its workspace from the reader keyed by :wsId, and reqscope — never set on
// that group — must not have displaced it.
func TestWorkspaceScopedRouteStillUsesTheReader(t *testing.T) {
	resolver := &recordingResolver{}
	reader := directReader{ws: domain.Workspace{ID: "ws-direct", WorktreePath: "/direct"}}
	r := identityRouterForScopes(t, resolver, reader, domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-direct/identity", http.NoBody)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, resolver.seenPaths)
	assert.Equal(t, "/direct", resolver.seenPaths[len(resolver.seenPaths)-1])
}
