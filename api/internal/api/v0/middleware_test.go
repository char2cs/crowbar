package v0

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeRepoReader struct {
	repo   *domain.Repository
	err    error
	called int
}

func (f *fakeRepoReader) FindByKey(_ context.Context, _ string) (*domain.Repository, error) {
	f.called++
	return f.repo, f.err
}

func mountRepoGuard(reader repoScopeReader) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/v0/projects/:projectId")
	grp.Use(scopeRepoToPath(reader))
	grp.GET("/repos/:repoId/thing", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	grp.DELETE("/repos/:repoId", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })
	grp.GET("/repos", func(c *gin.Context) { c.String(http.StatusOK, "list") })
	return r
}

// TestRegression_RepoScopeGuard_RejectsCrossProjectRepoId proves pass-4: a :repoId
// from a different project must 404, not let a destructive op (DeleteRepo, icon
// write) touch another project's repo.
func TestRegression_RepoScopeGuard_RejectsCrossProjectRepoId(t *testing.T) {
	reader := &fakeRepoReader{repo: &domain.Repository{ID: "R", ProjectID: "pX"}}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/v0/projects/pA/repos/R", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"a :repoId from a different project must 404 before the handler")
}

// TestRegression_RepoScopeGuard_AllowsMatchingProject proves an in-scope repo
// reaches the handler.
func TestRegression_RepoScopeGuard_AllowsMatchingProject(t *testing.T) {
	reader := &fakeRepoReader{repo: &domain.Repository{ID: "R", ProjectID: "pA"}}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/R/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRegression_RepoScopeGuard_404OnMissingRepo proves a non-existent repo 404s.
func TestRegression_RepoScopeGuard_404OnMissingRepo(t *testing.T) {
	reader := &fakeRepoReader{repo: nil}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/ghost/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestRegression_RepoScopeGuard_ReturnsMappedErrorOnReaderFailure proves a
// repository-lookup error (a storage read failure, not a missing repo) is
// mapped through libs.StatusAndMessage and aborts the chain, rather than
// falling through to the generic 404 a nil repo gets or reaching the handler.
func TestRegression_RepoScopeGuard_ReturnsMappedErrorOnReaderFailure(t *testing.T) {
	reader := &fakeRepoReader{err: errors.New("read model unavailable")}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/R/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a reader failure must abort with a mapped status, not reach the handler")
}

// TestRegression_RepoScopeGuard_SkipsWhenNoRepoId proves the repo collection
// route (no :repoId) passes through without a lookup.
func TestRegression_RepoScopeGuard_SkipsWhenNoRepoId(t *testing.T) {
	reader := &fakeRepoReader{repo: &domain.Repository{ID: "R", ProjectID: "pA"}}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, reader.called, "a route with no :repoId must not trigger a repo lookup")
}

type fakeScopeReader struct {
	ws     domain.Workspace
	err    error
	called int
}

func (f *fakeScopeReader) Get(_ context.Context, _ string) (domain.Workspace, error) {
	f.called++
	return f.ws, f.err
}

func mountScopeGuard(reader workspaceScopeReader) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/v0/projects/:projectId/repos/:repoId")
	grp.Use(scopeWorkspaceToPath(reader))
	grp.GET("/workspaces/:wsId/thing", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	grp.GET("/workspaces", func(c *gin.Context) { c.String(http.StatusOK, "list") })
	return r
}

// TestRegression_ScopeGuard_RejectsCrossScopeWsId proves pass-3 #1: a :wsId that
// belongs to a different project/repo must 404, not leak/mutate the workspace.
func TestRegression_ScopeGuard_RejectsCrossScopeWsId(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pX", RepoID: "rX"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"a :wsId from a different project/repo must 404 rather than reach the handler")
}

// TestRegression_ScopeGuard_RejectsRepoMismatchSameProject proves the repo
// segment is enforced even when the project matches.
func TestRegression_ScopeGuard_RejectsRepoMismatchSameProject(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pA", RepoID: "rX"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestRegression_ScopeGuard_AllowsMatchingScope proves an in-scope :wsId reaches
// the handler.
func TestRegression_ScopeGuard_AllowsMatchingScope(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pA", RepoID: "rA"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRegression_ScopeGuard_ReturnsMappedErrorOnReaderFailure proves a
// workspace-lookup error (a storage read failure, not a missing workspace) is
// mapped through libs.StatusAndMessage and aborts the chain, rather than
// falling through to the generic 404 a project/repo mismatch gets or reaching
// the handler.
func TestRegression_ScopeGuard_ReturnsMappedErrorOnReaderFailure(t *testing.T) {
	reader := &fakeScopeReader{err: errors.New("read model unavailable")}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a reader failure must abort with a mapped status, not reach the handler")
}

// TestRegression_ScopeGuard_SkipsWhenNoWsId proves collection/repo-level routes
// (no :wsId) pass through without a workspace lookup.
func TestRegression_ScopeGuard_SkipsWhenNoWsId(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pA", RepoID: "rA"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, reader.called, "a route with no :wsId must not trigger a workspace lookup")
}

func mountRejectEmptyPathParams() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rejectEmptyPathParams())
	r.GET("/v0/workspaces/:wsId/chats", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

// TestRejectEmptyPathParams_RejectsAnEmptySegment pins the fix this
// middleware exists for: gin's radix tree happily matches
// GET /v0/workspaces//chats against /v0/workspaces/:wsId/chats with wsId
// bound to "". Without this guard, an empty id would reach the handler and be
// treated as a real (if nonsensical) workspace id.
func TestRejectEmptyPathParams_RejectsAnEmptySegment(t *testing.T) {
	r := mountRejectEmptyPathParams()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/workspaces//chats", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "wsId", "the 400 must name which param was empty")
}

// TestRejectEmptyPathParams_AllowsANonEmptySegment proves the guard is not
// simply refusing every request through it: a normal, populated :wsId still
// reaches the handler.
func TestRejectEmptyPathParams_AllowsANonEmptySegment(t *testing.T) {
	r := mountRejectEmptyPathParams()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/workspaces/w1/chats", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
