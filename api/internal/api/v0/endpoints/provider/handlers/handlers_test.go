package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubEngine struct{}

func (stubEngine) PollOnView(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (engineprovider.ProviderState, error) {
	return engineprovider.ProviderState{}, nil
}

func (stubEngine) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return []string{"main"}, nil
}

type stubReader struct{}

func (stubReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return []domain.Workspace{
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo", Branch: "main"},
	}, nil
}

// newRouter wires provider's handlers onto the flat chat-scoped group and the
// repo-scoped group the way router.go does, with a stand-in for chatScoped's
// resolveChatWorktree middleware for State's route.
func newRouter(
	eng handlers.ProviderEngine,
	r handlers.WorkspaceReader,
) *gin.Engine {
	router := gin.New()
	h := handlers.New(eng, r)
	chatScoped := router.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, domain.Workspace{ID: c.Param("chatId"), WorktreePath: "/repo", Branch: "main"})
		c.Next()
	})
	chatScoped.GET("/provider", h.State)
	router.Group("/v0").GET("/repos/:repoId/protected-branches", h.ProtectedBranches)
	return router
}

// newRouterNoWorkspace wires the same State route WITHOUT resolveChatWorktree's
// stand-in ever running — the wiring-bug case Handlers.workspace guards
// against.
func newRouterNoWorkspace(
	eng handlers.ProviderEngine,
	r handlers.WorkspaceReader,
) *gin.Engine {
	router := gin.New()
	h := handlers.New(eng, r)
	router.Group("/v0/chats/:chatId").GET("/provider", h.State)
	return router
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

func TestProviderHandlers_HappyPath(
	t *testing.T,
) {
	r := newRouter(stubEngine{}, stubReader{})

	rec := do(r, "/v0/chats/chat1/provider")
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusOK, rec.Code)
}
