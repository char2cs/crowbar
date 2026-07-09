package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// TestMergeIntoParent_Returns202 asserts the fail-fast/good-path-async contract:
// a valid merge-into-parent passes synchronous validation (body shape, workspace
// exists), returns 202 with an empty body, and runs MergeIntoParent in the
// background. The merge outcome is delivered on the WebSocket stream (blackbox in
// W13), not in the HTTP response.
func TestMergeIntoParent_Returns202(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{
		mergeResult: worktree.MergeResult{ParentTipSha: "abc123"},
		mergeDone:   make(chan struct{}),
	}
	rec := do(
		newRouter(reader, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{"strategy":"squash"}`,
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.mergeDone)
	assert.Equal(t, "child", hierarchy.gotMergeID)
	assert.Equal(t, gitdomain.MergeStrategySquash, hierarchy.gotStrategy)
}

func TestMergeIntoParentBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMergeIntoParentMissingStrategy(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestMergeIntoParentMissingWorkspace_4xx asserts the synchronous existence
// check: a merge for an unknown workspace is rejected on the request path with a
// 4xx before any 202 or background merge.
func TestMergeIntoParentMissingWorkspace_4xx(
	t *testing.T,
) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	hierarchy := &fakeHierarchy{}
	rec := do(
		newRouter(reader, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{"strategy":"merge"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotMergeID, "merge must not run when the workspace is missing")
}

// TestMergeIntoParentAsyncErrorBroadcastsLastError asserts a background merge
// failure surfaces on the workspace entity via SetLastError, not on the HTTP
// response (the handler already returned 202).
func TestMergeIntoParentAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{mergeErr: worktree.ErrParentLocked}
	r := gin.New()
	lastErrors := &fakeLastErrors{called: make(chan struct{}, 1)}
	h := workspacehandlers.New(reader, hierarchy, &fakeRepos{}, lastErrors, fakeWork{})
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent", `{"strategy":"merge"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case <-lastErrors.called:
	case <-time.After(time.Second):
		t.Fatal("expected SetLastError to be called for the failed merge")
	}
	assert.Equal(t, "child", lastErrors.gotID)
}

// TestReparent_Returns202 asserts the fail-fast/good-path-async contract: a valid
// reparent passes synchronous validation (body shape, workspace exists), returns
// 202 with an empty body, and runs Reparent in the background. The reparented
// workspace is delivered on the WebSocket stream (blackbox in W13), not in the
// HTTP response.
func TestReparent_Returns202(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{
		reparented:   domain.Workspace{ID: "child", ParentID: "np"},
		reparentDone: make(chan struct{}),
	}
	rec := do(
		newRouter(reader, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{"newParentId":"np"}`,
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.reparentDone)
	assert.Equal(t, "child", hierarchy.gotReparent)
	assert.Equal(t, "np", hierarchy.gotNewParent)
}

func TestReparentBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReparentMissingNewParent(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestReparentMissingWorkspace_4xx asserts the synchronous existence check: a
// reparent for an unknown workspace is rejected on the request path with a 4xx
// before any 202 or background reparent.
func TestReparentMissingWorkspace_4xx(
	t *testing.T,
) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	hierarchy := &fakeHierarchy{}
	rec := do(
		newRouter(reader, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{"newParentId":"np"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotReparent, "reparent must not run when the workspace is missing")
}

// TestReparentAsyncErrorBroadcastsLastError asserts a background reparent failure
// surfaces on the workspace entity via SetLastError, not on the HTTP response.
func TestReparentAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{reparentErr: worktree.ErrChildHasChildren}
	r := gin.New()
	lastErrors := &fakeLastErrors{called: make(chan struct{}, 1)}
	h := workspacehandlers.New(reader, hierarchy, &fakeRepos{}, lastErrors, fakeWork{})
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/child/reparent", `{"newParentId":"np"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case <-lastErrors.called:
	case <-time.After(time.Second):
		t.Fatal("expected SetLastError to be called for the failed reparent")
	}
	assert.Equal(t, "child", lastErrors.gotID)
}

func TestRetryProvision_Returns202(t *testing.T) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/retry-provision",
		"",
	)
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestDetachHolder_Returns202(t *testing.T) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/detach-holder",
		"",
	)
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestRetryProvisionMissingWorkspace_4xx(t *testing.T) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/nope/retry-provision",
		"",
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
