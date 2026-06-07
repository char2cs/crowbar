package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

type stubProvider struct{}

func (stubProvider) PollOnView(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (providertypes.ProviderState, error) {
	return providertypes.ProviderState{}, nil
}

func (stubProvider) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return nil, nil
}

type stubWSReader struct{}

func (stubWSReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{WorktreePath: "/repo"}, nil
}

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	provider.Register(
		r.Group("/v0"),
		stubProvider{},
		stubWSReader{},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/provider", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	want := []string{
		"/v0/workspaces/:wsId/provider",
		"/v0/repos/:id/protected-branches",
	}
	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Path] = true
	}
	for _, path := range want {
		assert.True(t, mounted[path], "route %s not mounted", path)
	}
}
