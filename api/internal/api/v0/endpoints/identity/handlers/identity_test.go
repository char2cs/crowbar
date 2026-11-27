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
	r := newRouter(resolver, domain.Workspace{ID: "ws1", WorktreePath: "/repo/ws1", Provisioning: domain.WorkspaceProvisioned})

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
