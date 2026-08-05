package handlers_test

import (
	"errors"
	"net/http"
	"testing"

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

// TestImport_Returns202AndForwardsBranches asserts the batch-import contract:
// valid branches pass synchronous validation, return 202 with an empty body, and
// CreateFromImport runs in the background with the repo context and branch set.
func TestImport_Returns202AndForwardsBranches(t *testing.T) {
	hierarchy := &fakeHierarchy{importDone: make(chan struct{})}
	repos := &fakeRepos{repo: &domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Path:          "/repo",
		RemoteURL:     "https://github.com/test/repo.git",
		DefaultBranch: "main",
	}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/import",
		`{"branches":["feat/child","feat/base"]}`,
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.importDone)
	assert.Equal(t, "r1", hierarchy.gotImport.RepoID)
	assert.Equal(t, "p1", hierarchy.gotImport.ProjectID)
	assert.Equal(t, "/repo", hierarchy.gotImport.RepoPath)
	assert.Equal(t, "main", hierarchy.gotImport.DefaultBranch)
	assert.Equal(t, []string{"feat/child", "feat/base"}, hierarchy.gotImport.Branches)
}

// TestImport_DefaultBranch409 asserts the second fail-fast guard. The import
// chain walk terminates AT the default branch, so a request naming it produced
// no node, no workspace, and no error — while the client's optimistic row waits
// for a workspace that is never coming. A 202 for work that is defined to do
// nothing is a silent hang; refuse it synchronously so the caller's rejection
// path surfaces it.
func TestImport_DefaultBranch409(t *testing.T) {
	hierarchy := &fakeHierarchy{}
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/import",
		`{"branches":["feat/child","main"]}`,
	)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "main")
	assert.Empty(t, hierarchy.gotImport.Branches, "the batch must not be dispatched at all")
}

// TestImport_EmptyBranches400 asserts the fail-fast guard: an empty branch set
// is rejected synchronously and never dispatches background work.
func TestImport_EmptyBranches400(t *testing.T) {
	hierarchy := &fakeHierarchy{}
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/import",
		`{"branches":[]}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
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

// A create started on a folder row carries the folder and NO parent: the row is
// filed under the folder while it forks from the repo's default branch. The two
// fields must not be confused, because a folder has no branch to fork from.
func TestCreate_IntoFolder_FilesTheRowAndForksFromTheDefaultBranch(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "new-ws"}, createDone: make(chan struct{})}
	placer := &fakePlacer{folders: []domain.Folder{{ID: "f1", RepoID: "r1", ProjectID: "p1"}}}
	r, _, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, repos, placer)

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","folderId":"f1"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.createDone)
	assert.Equal(t, "f1", hierarchy.gotCreate.FolderID)
	assert.Empty(t, hierarchy.gotCreate.ParentID, "a folder is never a fork parent")
	assert.Equal(t, "main", hierarchy.gotCreate.ParentBranch)
}

// A folderId nothing renders is refused the way an unresolvable parentId is:
// synchronously, before the 202, where the caller can still see it. The repo's
// folder list is repo-scoped, so a folder owned by another repo takes the same
// answer.
func TestCreate_UnknownFolder_404(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	hierarchy := &fakeHierarchy{}
	placer := &fakePlacer{folders: []domain.Folder{{ID: "f1", RepoID: "r1", ProjectID: "p1"}}}
	r, _, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, repos, placer)

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","folderId":"from-another-repo"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotCreate.Branch, "a refused create must not dispatch background work")
}

// A created row is GIVEN its slot. Folders and fork-root workspaces share one
// sibling space, so a row left holding the zero value collides with whatever
// already sits at 0 and surfaces at the top of a level it should have joined at
// the end.
func TestCreate_AsksForTheNextSlotInTheLevelItJoins(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "new-ws"}, createDone: make(chan struct{})}
	placer := &fakePlacer{
		folders:  []domain.Folder{{ID: "f1", RepoID: "r1", ProjectID: "p1"}},
		nextSlot: 3,
	}
	r, _, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, repos, placer)

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","folderId":"f1"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.createDone)
	assert.Equal(t, 3, hierarchy.gotCreate.Order)
	assert.Equal(t, "f1", placer.gotContainer,
		"a fork root is ordered within the folder it names")
}

// A forked child is ordered among its fork siblings, so the level it joins is
// its parent — never the folder, which it does not carry.
func TestCreate_UnderAParent_CountsTheParentsLevel(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "new-ws"}, createDone: make(chan struct{})}
	reader := &fakeReader{get: domain.Workspace{ID: "w1", Branch: "feature/parent"}}
	placer := &fakePlacer{nextSlot: 1}
	r, _, _ := newRouterWithPlacer(reader, hierarchy, repos, placer)

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","parentId":"w1"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.createDone)
	assert.Equal(t, "w1", placer.gotContainer)
	assert.Equal(t, 1, hierarchy.gotCreate.Order)
}

// The two fields address different things, and a row can only be placed by one
// of them. Accepting both would let a create reach a state the placement
// endpoint refuses outright.
func TestCreate_ParentAndFolderTogether_400(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	hierarchy := &fakeHierarchy{}
	placer := &fakePlacer{folders: []domain.Folder{{ID: "f1", RepoID: "r1", ProjectID: "p1"}}}
	r, _, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, repos, placer)

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","parentId":"w1","folderId":"f1"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotCreate.Branch, "a refused create must not dispatch background work")
}

func TestCreate_FolderLookupError_500(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	placer := &fakePlacer{listErr: errors.New("db down")}
	r, _, _ := newRouterWithPlacer(&fakeReader{}, &fakeHierarchy{}, repos, placer)

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","folderId":"f1"}`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// With no folder usecase wired there is nothing to resolve the destination
// against, so the create is refused rather than silently dropping the folder and
// filing the row at the repo root.
func TestCreate_IntoFolderWithoutPlacer_500(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main",
	}}
	hierarchy := &fakeHierarchy{}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"feat","folderId":"f1"}`,
	)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, hierarchy.gotCreate.Branch)
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
	// The SetLastError call IS the signal that the failed cascade surfaced on the
	// entity; block on it rather than guessing at a duration.
	<-lastErrors.called
	assert.Equal(t, "w1", lastErrors.gotID)
	assert.Equal(t, "boom", lastErrors.gotMsg)
}
