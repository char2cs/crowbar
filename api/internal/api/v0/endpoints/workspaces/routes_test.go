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
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
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

func (stubReader) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
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

func (stubReader) MergeEligibilityFor(
	_ context.Context,
	_ domain.Workspace,
	_ []domain.Workspace,
) workspace.MergeEligibility {
	return workspace.MergeEligibility{}
}

type stubHierarchy struct{}

func (stubHierarchy) CreateChild(
	_ context.Context,
	_ workspace.CreateChildInput,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubHierarchy) CreateFromImport(
	_ context.Context,
	_ workspace.ImportInput,
) error {
	return nil
}

func (stubHierarchy) MergeIntoParent(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) (workspace.MergeResult, error) {
	return workspace.MergeResult{}, nil
}

func (stubHierarchy) RebaseOntoParent(
	_ context.Context,
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

func (stubHierarchy) RetryProvision(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubHierarchy) RenameBranch(
	_ context.Context,
	_ string,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubHierarchy) DetachHolder(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
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

type stubWork struct{}

func (stubWork) BeginWork(_ context.Context, _ string) {}
func (stubWork) EndWork(_ context.Context, _ string)   {}
func (stubWork) IsWorking(_ string) bool               { return false }
func (stubWork) WorkingFor(_ string) bool              { return false }

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
		stubWork{},
		nil,
		nil,
		nil,
		nil,
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
		{http.MethodPatch, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/merge-into-parent"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
	assert.False(t, wsHit)
}

// TestWorkspaceRoutes_ReparentRemoved asserts the standalone reparent route is
// gone: its git-lineage effect is now reached only through the hierarchy
// usecase's guarded Reparent, never exposed as its own route.
func TestWorkspaceRoutes_ReparentRemoved(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	workspaces.Register(
		repoScoped,
		stubReader{},
		stubHierarchy{},
		stubRepos{},
		stubLastErrors{},
		stubWork{},
		nil,
		nil,
		nil,
		nil,
		func(_ *gin.Context) {},
		passthrough,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/reparent", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestWorkspaceRoutes_NoRouteExposesReparent walks every route the workspaces
// endpoint mounts and asserts none of them is reparent-shaped: the guarded
// hierarchy.Reparent usecase method has no HTTP surface left at all, not
// merely a renamed one.
func TestWorkspaceRoutes_NoRouteExposesReparent(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	workspaces.Register(
		repoScoped,
		stubReader{},
		stubHierarchy{},
		stubRepos{},
		stubLastErrors{},
		stubWork{},
		nil,
		nil,
		nil,
		nil,
		func(_ *gin.Context) {},
		passthrough,
	)
	for _, route := range r.Routes() {
		assert.NotContains(t, route.Path, "reparent")
	}
}

func (stubReader) SetLock(
	_ context.Context,
	id string,
	locked *bool,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}
