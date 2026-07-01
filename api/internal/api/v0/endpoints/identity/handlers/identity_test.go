package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity/handlers"
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

// fakeWsReader resolves a workspace by id, or fails when getErr is set.
type fakeWsReader struct {
	ws     domain.Workspace
	getErr error
}

func (f *fakeWsReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	if f.getErr != nil {
		return domain.Workspace{}, f.getErr
	}
	return f.ws, nil
}

func newRouter(
	resolver handlers.IdentityResolver,
	wsReader handlers.WorkspaceReader,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(resolver, wsReader)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/identity", h.Get)
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

// TestGet_WorkspaceNotFound pins that an unknown workspace id degrades to a
// best-effort 200 with every identity field empty, rather than a 404.
func TestGet_WorkspaceNotFound(
	t *testing.T,
) {
	resolver := &fakeResolver{}
	wsReader := &fakeWsReader{getErr: errors.New("no such workspace")}
	r := newRouter(resolver, wsReader)

	rec := do(r, "/v0/workspaces/missing/identity")

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
	assert.Empty(t, resolver.gotWtPath, "resolver must not be invoked when the workspace lookup fails")
}

// TestGet_Found pins that a known workspace resolves the identity from its
// worktree path and serves it back on the query envelope.
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
	wsReader := &fakeWsReader{ws: domain.Workspace{ID: "ws1", WorktreePath: "/repo/ws1"}}
	r := newRouter(resolver, wsReader)

	rec := do(r, "/v0/workspaces/ws1/identity")

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
