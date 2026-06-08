package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
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

func (stubReader) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: "/repo", Branch: "main"}, nil
}

func newRouter(
	eng handlers.ProviderEngine,
	r handlers.WorkspaceReader,
) *gin.Engine {
	router := gin.New()
	h := handlers.New(eng, r)
	rg := router.Group("/v0")
	rg.GET("/workspaces/:wsId/provider", h.State)
	rg.GET("/repos/:id/protected-branches", h.ProtectedBranches)
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

	rec := do(r, "/v0/workspaces/ws1/provider")
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusOK, rec.Code)
}
