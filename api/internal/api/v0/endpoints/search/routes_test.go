package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubEngine struct{}

func (stubEngine) Search(
	_ context.Context,
	_ string,
	_ enginesearch.SearchRequest,
) (enginesearch.SearchResponse, error) {
	return enginesearch.SearchResponse{}, nil
}

func (stubEngine) Replace(
	_ context.Context,
	_ string,
	_ enginesearch.ReplaceRequest,
	_ bool,
) error {
	return nil
}

// searchSurface is the method+relative-path set search.Register mounts,
// written once (mirroring git's routes_test.go, the reference implementation
// for this step).
func searchSurface() []struct {
	method string
	path   string
} {
	return []struct {
		method string
		path   string
	}{
		{http.MethodPost, ""},
		{http.MethodPost, "/replace"},
	}
}

// registerChatScoped wires search.Register the way router.go does: on the
// flat chat-scoped group alone (spec §8 step 6 retired the old
// workspace-scoped mount), with a stand-in for chatScoped's
// resolveChatWorktree middleware so a mounted route resolves a workspace the
// same way production does.
func registerChatScoped(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	chatScoped := v0.Group("/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, domain.Workspace{ID: c.Param("chatId"), WorktreePath: "/repo"})
		c.Next()
	})
	search.Register(chatScoped, stubEngine{})
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every
// search route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(
	t *testing.T,
) {
	r := registerChatScoped(t)

	for _, tc := range searchSurface() {
		path := "/v0/chats/chat1/search" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterDropsWorkspaceScopedRoutes proves spec §8 step 6's deletion is
// real for search: the old /v0/workspaces/:wsId/search... mount, kept alive
// alongside the chat-scoped one through the rest of this refactor, answers
// nothing any more.
func TestRegisterDropsWorkspaceScopedRoutes(
	t *testing.T,
) {
	r := registerChatScoped(t)

	for _, tc := range searchSurface() {
		path := "/v0/workspaces/ws1/search" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}
