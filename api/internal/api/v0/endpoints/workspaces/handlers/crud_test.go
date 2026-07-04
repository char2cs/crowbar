package handlers_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestCreate_Returns202 asserts the fail-fast/good-path-async contract: a valid
// create passes synchronous validation, returns 202 with an empty body, and
// runs CreateChild in the background (00 §4). The created workspace is delivered
// on the WebSocket stream (blackbox in W13), not in the HTTP response.
func TestCreate_Returns202(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "new-ws"}, createDone: make(chan struct{})}
	repos := &fakeRepos{repo: &domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Path:          "/repo",
		DefaultBranch: "main",
	}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat"}`,
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.createDone)
	assert.Equal(t, "r1", hierarchy.gotCreate.RepoID)
	assert.Equal(t, "p1", hierarchy.gotCreate.ProjectID)
	assert.Equal(t, "/repo", hierarchy.gotCreate.RepoPath)
	assert.Equal(t, "feat", hierarchy.gotCreate.Branch)
	assert.Equal(t, "main", hierarchy.gotCreate.ParentBranch)
	assert.Empty(t, hierarchy.gotCreate.ParentID)
}

func TestCreateSuccessFromParent(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "child"}, createDone: make(chan struct{})}
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main"}}
	reader := &fakeReader{get: domain.Workspace{ID: "parent", Branch: "parent-branch"}}
	rec := do(
		newRouter(reader, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","parentId":"parent"}`,
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.createDone)
	assert.Equal(t, "parent", hierarchy.gotCreate.ParentID)
	assert.Equal(t, "parent-branch", hierarchy.gotCreate.ParentBranch)
	assert.Equal(t, "parent", reader.gotID)
}

func TestCreateBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestCreateMissingRepoID exercises the empty-repoId guard directly: the Create
// handler is mounted without the :repoId path param so c.Param("repoId") is "".
func TestCreateMissingRepoID(
	t *testing.T,
) {
	h := workspacehandlers.New(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}, &fakeLastErrors{}, fakeWork{})
	r := gin.New()
	r.POST("/v0/workspaces", h.Create)
	rec := do(r, http.MethodPost, "/v0/workspaces", `{"branch":"feat"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateMissingBranch(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateRepoNotFound(
	t *testing.T,
) {
	repos := &fakeRepos{repo: nil}
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/missing/workspaces",
		`{"branch":"feat"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCreateRepoLookupError(
	t *testing.T,
) {
	repos := &fakeRepos{err: errors.New("db down")}
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat"}`,
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCreateParentLookupNotFound(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}}
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","parentId":"nope"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestCreate_ValidationFailsSync_4xx asserts the synchronous fail-fast path:
// a missing branch or an unresolvable repo is rejected with a 4xx on the
// request path, before any 202 or background work (00 §4).
func TestCreate_ValidationFailsSync_4xx(
	t *testing.T,
) {
	cases := []struct {
		name string
		body string
		repo *domain.Repository
		want int
	}{
		{
			name: "missing branch",
			body: `{}`,
			repo: &domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"},
			want: http.StatusBadRequest,
		},
		{
			name: "repo not found",
			body: `{"branch":"feat"}`,
			repo: nil,
			want: http.StatusNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hierarchy := &fakeHierarchy{}
			rec := do(
				newRouter(&fakeReader{}, hierarchy, &fakeRepos{repo: tc.repo}),
				http.MethodPost,
				"/v0/projects/p1/repos/r1/workspaces",
				tc.body,
			)
			assert.Equal(t, tc.want, rec.Code)
			assert.Empty(t, hierarchy.gotCreate.Branch, "create must not run when validation fails")
		})
	}
}

// TestCreateAsyncErrorReturns202 asserts a CreateChild failure does NOT surface
// on the HTTP response: validation passed, so the handler already returned 202
// and the error is handled in the background.
func TestCreateAsyncErrorReturns202(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{createErr: errors.New("boom"), createDone: make(chan struct{})}
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat"}`,
	)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.createDone)
}

// TestDelete_Returns202 asserts the delete fail-fast/good-path-async contract:
// the workspace is validated to exist synchronously, then 202 is returned and
// DeleteCascade runs in the background. The deleted tombstone is broadcast on
// the WebSocket stream (blackbox in W13), not in the HTTP response.
func TestDelete_Returns202(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{deleteDone: make(chan struct{})}
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	rec := do(
		newRouter(reader, hierarchy, &fakeRepos{}),
		http.MethodDelete,
		"/v0/projects/p1/repos/r1/workspaces/w1",
		"",
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.deleteDone)
	assert.Equal(t, "w1", hierarchy.gotDeleteID)
}

// TestDeleteMissingWorkspace_4xx asserts the synchronous existence check: a
// delete for an unknown workspace is rejected on the request path with a 4xx
// before any 202 or background cascade.
func TestDeleteMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	rec := do(
		newRouter(reader, hierarchy, &fakeRepos{}),
		http.MethodDelete,
		"/v0/projects/p1/repos/r1/workspaces/missing",
		"",
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotDeleteID, "cascade must not run when the workspace is missing")
}

// A MANAGED workspace already on the requested branch → synchronous 409.
func TestCreate_DuplicateBranch_Returns409(t *testing.T) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	reader := &fakeReader{list: []domain.Workspace{{ID: "w1", RepoID: "r1", Branch: "develop"}}}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"develop"}`,
	)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// The DEFAULT workspace (the imported repo folder) must NOT block importing its
// branch — it is unmanaged, so the sync 409 guard skips it and the create is
// accepted (202). The usecase/git layer decides whether the worktree can be added.
func TestCreate_DefaultBranchWorkspace_NotBlocked(t *testing.T) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "develop",
	}}
	reader := &fakeReader{list: []domain.Workspace{
		{ID: "def", RepoID: "r1", Branch: "develop", IsDefault: true},
	}}
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "new-ws"}, createDone: make(chan struct{})}
	rec := do(
		newRouter(reader, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"develop"}`,
	)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.createDone)
}

func TestDeleteAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{deleteErr: errors.New("boom")}
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	r := gin.New()
	lastErrors := &fakeLastErrors{called: make(chan struct{}, 1)}
	h := workspacehandlers.New(reader, hierarchy, &fakeRepos{}, lastErrors, fakeWork{})
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rec := do(r, http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/w1", "")

	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case <-lastErrors.called:
	case <-time.After(time.Second):
		t.Fatal("expected SetLastError to be called for the failed cascade")
	}
	assert.Equal(t, "w1", lastErrors.gotID)
	assert.Equal(t, "boom", lastErrors.gotMsg)
}
