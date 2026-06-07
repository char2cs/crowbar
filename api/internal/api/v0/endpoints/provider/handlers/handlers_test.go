package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	providerhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

var errBoom = errors.New("boom")

type fakeProvider struct {
	state        providertypes.ProviderState
	branches     []string
	pollErr      error
	branchErr    error
	lastRepoPath string
}

func (f *fakeProvider) PollOnView(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (providertypes.ProviderState, error) {
	return f.state, f.pollErr
}

func (f *fakeProvider) ProtectedBranches(
	_ context.Context,
	repoPath string,
) ([]string, error) {
	f.lastRepoPath = repoPath
	return f.branches, f.branchErr
}

var _ providerhandlers.ProviderEngine = (*fakeProvider)(nil)

type fakeWSReader struct {
	ws  domain.Workspace
	err error
}

func (f *fakeWSReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	if f.err != nil {
		return domain.Workspace{}, f.err
	}
	return f.ws, nil
}

var _ providerhandlers.WorkspaceReader = (*fakeWSReader)(nil)

type fakeRepoReader struct {
	repo *domain.Repository
	err  error
}

func (f *fakeRepoReader) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repo, nil
}

var _ providerhandlers.RepoReader = (*fakeRepoReader)(nil)

func okWSReader() *fakeWSReader {
	return &fakeWSReader{
		ws: domain.Workspace{
			ID:           "ws1",
			Branch:       "feature",
			WorktreePath: "/repo",
		},
	}
}

func okRepoReader() *fakeRepoReader {
	return &fakeRepoReader{
		repo: &domain.Repository{
			ID:   "r1",
			Path: "/repo-root",
		},
	}
}

func newRouter(
	prov providerhandlers.ProviderEngine,
	wsReader providerhandlers.WorkspaceReader,
) *gin.Engine {
	return newRouterWithRepos(prov, wsReader, okRepoReader())
}

func newRouterWithRepos(
	prov providerhandlers.ProviderEngine,
	wsReader providerhandlers.WorkspaceReader,
	repos providerhandlers.RepoReader,
) *gin.Engine {
	r := gin.New()
	h := providerhandlers.New(prov, wsReader, repos)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/provider", h.ProviderState)
	rg.GET("/repos/:id/protected-branches", h.ProtectedBranches)
	return r
}

func do(
	t *testing.T,
	r *gin.Engine,
	target string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	r.ServeHTTP(rec, req)
	return rec
}

type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func decode(
	t *testing.T,
	rec *httptest.ResponseRecorder,
) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env
}
