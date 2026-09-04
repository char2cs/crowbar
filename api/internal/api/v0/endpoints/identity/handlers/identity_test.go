package handlers_test

import (
	"context"
	"encoding/json"
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

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// fakeResolver returns a canned identity, or records the worktree path it was
// called with.
type fakeResolver struct {
	identity  gitdomain.Identity
	gotWtPath string
}

func (f *fakeResolver) CurrentIdentity(
	_ context.Context,
	worktreePath string,
) gitdomain.Identity {
	f.gotWtPath = worktreePath
	return f.identity
}

// newRouter wires the identity handler onto the flat chat-scoped group the
// way router.go does, with a stand-in for chatScoped's resolveChatWorktree
// middleware that always resolves ws.
func newRouter(
	resolver handlers.IdentityResolver,
	ws domain.Workspace,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(resolver)
	rg := r.Group("/v0/chats/:chatId")
	rg.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, ws)
		c.Next()
	})
	rg.GET("/identity", h.Get)
	return r
}

// newRouterNoWorkspace wires the same route WITHOUT resolveChatWorktree's
// stand-in ever running, so resolveWorkspace's reqscope read comes back
// empty — the same degradation an unresolvable workspace id used to produce
// before spec §8 step 6 retired the WorkspaceReader fallback.
func newRouterNoWorkspace(
	resolver handlers.IdentityResolver,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(resolver)
	r.Group("/v0/chats/:chatId").GET("/identity", h.Get)
	return r
}

func do(
	r *gin.Engine,
	path string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}

// TestGet_NoResolvedWorkspace pins that a request reaching this handler with
// no workspace resolved on reqscope degrades to a best-effort 200 with every
// identity field empty, rather than a 404 or 500.
func TestGet_NoResolvedWorkspace(
	t *testing.T,
) {
	resolver := &fakeResolver{}
	r := newRouterNoWorkspace(resolver)

	rec := do(r, "/v0/chats/chat1/identity")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Login       string `json:"login"`
			DisplayName string `json:"displayName"`
			AvatarURL   string `json:"avatarUrl"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Empty(t, body.Data.Login)
	assert.Empty(t, body.Data.DisplayName)
	assert.Empty(t, body.Data.AvatarURL)
	assert.Empty(t, resolver.gotWtPath, "resolver must not be invoked when no workspace is resolved")
}

// TestGet_Found pins that a resolved chat worktree resolves the identity from
// its worktree path and serves it back on the query envelope.
func TestGet_Found(
	t *testing.T,
) {
	resolver := &fakeResolver{
		identity: gitdomain.Identity{
			Login:       "octocat",
			DisplayName: "The Octocat",
			AvatarURL:   "https://example.com/avatar.png",
		},
	}
	r := newRouter(resolver, domain.Workspace{ID: "ws1", WorktreePath: "/repo/ws1"})

	rec := do(r, "/v0/chats/chat1/identity")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/repo/ws1", resolver.gotWtPath)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Login       string `json:"login"`
			DisplayName string `json:"displayName"`
			AvatarURL   string `json:"avatarUrl"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "octocat", body.Data.Login)
	assert.Equal(t, "The Octocat", body.Data.DisplayName)
	assert.Equal(t, "https://example.com/avatar.png", body.Data.AvatarURL)
}
