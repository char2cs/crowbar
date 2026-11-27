package worktree_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
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

type stubWorktrees struct{}

func (stubWorktrees) Resolve(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: "abc"}, nil
}

type stubReader struct{}

func (stubReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, nil
}

func (stubReader) SyncWorkingTreeState(
	_ context.Context,
	_ string,
	_ time.Time,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (stubReader) SetLock(
	_ context.Context,
	id string,
	locked *bool,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}

type stubHierarchy struct{}

func (stubHierarchy) CreateFromImport(
	_ context.Context,
	_ workspace.ImportInput,
	_ workspace.ImportProgress,
) error {
	return nil
}

func (stubHierarchy) GuardMergeFold(context.Context, string) error { return nil }

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
	_ domain.DeleteConsent,
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

type stubOps struct{}

func (stubOps) Start(
	_ context.Context,
	spec operation.Spec,
	_ operation.Work,
) (domain.Operation, error) {
	return domain.Operation{ID: spec.ID, Kind: spec.Kind}, nil
}

func registerOn(
	r *gin.Engine,
) {
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	worktree.Register(
		repoScoped,
		stubReader{},
		stubHierarchy{},
		stubRepos{},
		stubOps{},
		nil,
		nil,
		stubWorktrees{},
	)
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	registerOn(r)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/lock"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/sync"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/merge-into-parent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/reparent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/rebase-onto-parent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/retry-provision"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/detach-holder"},
		{http.MethodPatch, "/v0/projects/p1/repos/r1/chats/c1/branch"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/chats/import-batch"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}

func TestRegisterMountsNoWorkspaceKeyedRoutes(
	t *testing.T,
) {
	r := gin.New()
	registerOn(r)

	gone := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/projects/p1/repos/r1/workspaces"},
		{http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/import"},
		{http.MethodPatch, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/abc"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/sync"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/lock"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/merge-into-parent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/reparent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/rebase-onto-parent"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/retry-provision"},
		{http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/abc/detach-holder"},
	}
	assert.Len(t, gone, 13, "the group mounted thirteen :wsId routes; all thirteen must be checked")
	for _, tc := range gone {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, tc.path)
	}
}

func TestRegisterWithoutAResolverMountsOnlyTheImport(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	worktree.Register(
		repoScoped,
		stubReader{},
		stubHierarchy{},
		stubRepos{},
		stubOps{},
		nil,
		nil,
		nil,
	)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/lock", http.NoBody))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost, "/v0/projects/p1/repos/r1/chats/import-batch", http.NoBody))
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}
