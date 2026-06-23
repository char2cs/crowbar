package v0

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
