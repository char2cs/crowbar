package workspaces_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubReader struct{}

func (stubReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, nil
}

func (stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubReader) SyncWorkingTreeState(
	_ context.Context,
	_ string,
	_ time.Time,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

type stubHierarchy struct{}

func (stubHierarchy) CreateChild(
	_ context.Context,
	_ worktree.CreateChildInput,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubHierarchy) MergeIntoParent(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) (worktree.MergeResult, error) {
	return worktree.MergeResult{}, nil
}

func (stubHierarchy) Reparent(
	_ context.Context,
	_ string,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubHierarchy) DeleteCascade(
	_ context.Context,
	_ string,
) error {
	return nil
}

type stubRepos struct{}

func (stubRepos) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	return nil, nil
}

type stubLastErrors struct{}

func (stubLastErrors) SetLastError(
	_ context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LastError: message}, nil
}

func passthrough(
	rest gin.HandlerFunc,
	_ gin.HandlerFunc,
) gin.HandlerFunc {
	return rest
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	var wsHit bool
	// workspaces.Register mounts on the repo-scoped group, so build the
	// hierarchical prefix to mirror the production router chain.
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	workspaces.Register(
		repoScoped,
		stubReader{},
		stubHierarchy{},
		stubRepos{},
		stubLastErrors{},
		func(_ *gin.Context) { wsHit = true },
		passthrough,
	)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects/p1/repos/r1/workspaces"},
		{http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces"},
		{http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/merge-into-parent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/reparent"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
	assert.False(t, wsHit)
}
