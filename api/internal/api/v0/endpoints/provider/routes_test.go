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
	// provider.Register mounts on the repo-scoped group and the flat
	// chat-scoped group, so build both prefixes to mirror the production
	// router chain.
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	chatScoped := r.Group("/v0/chats/:chatId")
	provider.Register(repoScoped, chatScoped, stubEngine{}, stubReader{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/provider"},
		{http.MethodGet, "/v0/projects/p1/repos/r1/protected-branches"},
		{http.MethodGet, "/v0/chats/chat1/provider"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}

// TestRegister_ProtectedBranchesDoesNotMoveToChatScope pins spec §4.2's
// explicit carve-out: /protected-branches is repo-level, not worktree-owned,
// so it is registered ONLY on the repo-scoped group, never on the chat-scoped
// one — unlike State, which this step moves onto both.
func TestRegister_ProtectedBranchesDoesNotMoveToChatScope(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	chatScoped := r.Group("/v0/chats/:chatId")
	provider.Register(repoScoped, chatScoped, stubEngine{}, stubReader{})

	for _, route := range r.Routes() {
		assert.NotContains(
			t, route.Path, "/chats/:chatId/protected-branches",
			"protected-branches must not be chat-scoped",
		)
	}
}
