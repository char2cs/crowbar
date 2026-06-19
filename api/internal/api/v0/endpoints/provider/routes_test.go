package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider"
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
	return nil, nil
}

type stubReader struct{}

func (stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return []domain.Workspace{{ID: "ws1", RepoID: "r1"}}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	// provider.Register mounts on the repo-scoped group, so build the
	// hierarchical prefix to mirror the production router chain.
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	provider.Register(repoScoped, stubEngine{}, stubReader{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/provider"},
		{http.MethodGet, "/v0/projects/p1/repos/r1/protected-branches"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
