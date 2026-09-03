package search_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
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

type stubReader struct{}

func (stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

// searchSurface is the method+relative-path set search.Register mounts,
// written once and asserted against BOTH live prefixes (mirroring git's
// routes_test.go, the reference implementation for this step).
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

// registerBothMounts wires search.Register the way router.go does: the old
// workspace-scoped group and the flat chat-scoped one, on one engine.
func registerBothMounts(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	search.Register(v0, v0.Group("/chats/:chatId"), stubEngine{}, stubReader{})
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every
// search route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(
	t *testing.T,
) {
	r := registerBothMounts(t)

	for _, tc := range searchSurface() {
		path := "/v0/chats/chat1/search" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterKeepsWorkspaceScopedRoutes is the regression bar for the
// coexistence this step deliberately ships: the workspace-scoped surface is
// NOT retired here (spec §8 step 6 does that, once every group has moved), so
// every one of its routes must still answer exactly as before.
func TestRegisterKeepsWorkspaceScopedRoutes(
	t *testing.T,
) {
	r := registerBothMounts(t)

	for _, tc := range searchSurface() {
		path := "/v0/workspaces/ws1/search" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}
