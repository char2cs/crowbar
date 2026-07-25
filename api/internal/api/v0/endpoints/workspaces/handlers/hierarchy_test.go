package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	// The SetLastError call IS the signal that the failed merge surfaced on the
	// entity; block on it rather than guessing at a duration.
	<-lastErrors.called
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
	// The SetLastError call IS the signal that the failed reparent surfaced on
	// the entity; block on it rather than guessing at a duration.
	<-lastErrors.called
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

// newFoldRouter builds the handler directly (rather than via newRouter) so tests
// can reach WaitAsync: the post-merge fold runs in the detached runAsync
// goroutine, and WaitAsync is the only sound way to assert a NEGATIVE ("the
// non-leaf child was NOT deleted") — a sleep would pass on a fast machine for the
// wrong reason.
func newFoldRouter(
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	lastErrors workspacehandlers.LastErrorSetter,
) (*gin.Engine, *workspacehandlers.Handlers) {
	r := gin.New()
	h := workspacehandlers.New(reader, hierarchy, &fakeRepos{}, lastErrors, fakeWork{})
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/rebase-onto-parent", h.RebaseOntoParent)
	return r, h
}

func mergeInto(
	r *gin.Engine,
	body string,
) *httptest.ResponseRecorder {
	return do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent", body)
}

// TestMergeIntoParent_DeleteSourceFoldsMergedLeaf asserts the fold: a clean merge
// with deleteSource removes the child once it is a LEAF, because at that point its
// work lives in the parent and nothing else descends from it.
func TestMergeIntoParent_DeleteSourceFoldsMergedLeaf(
	t *testing.T,
) {
	reader := &fakeReader{
		get: domain.Workspace{ID: "child"},
		// Siblings under the same parent do not make "child" a non-leaf: only a
		// workspace whose ParentID IS "child" would.
		list: []domain.Workspace{
			{ID: "child", ParentID: "parent"},
			{ID: "sibling", ParentID: "parent"},
		},
	}
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ParentTipSha: "abc123"}}
	r, h := newFoldRouter(reader, hierarchy, &fakeLastErrors{})

	rec := mergeInto(r, `{"strategy":"squash","deleteSource":true}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Equal(t, "child", hierarchy.gotDeleteID, "a merged leaf is folded away")
}

// TestMergeIntoParent_DeleteSourceKeepsNonLeaf asserts the no-silent-data-loss
// rule: a merged child that still has descendants is KEPT, because DeleteCascade
// would take its children's unmerged work with it.
func TestMergeIntoParent_DeleteSourceKeepsNonLeaf(
	t *testing.T,
) {
	reader := &fakeReader{
		get: domain.Workspace{ID: "child"},
		list: []domain.Workspace{
			{ID: "child", ParentID: "parent"},
			{ID: "grandchild", ParentID: "child"},
		},
	}
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ParentTipSha: "abc123"}}
	lastErrors := &fakeLastErrors{}
	r, h := newFoldRouter(reader, hierarchy, lastErrors)

	rec := mergeInto(r, `{"strategy":"squash","deleteSource":true}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Empty(t, hierarchy.gotDeleteID, "a child with descendants must survive the merge")
	assert.Empty(t, lastErrors.gotMsg, "keeping a non-leaf is a no-op, not a failure")
}

// TestMergeIntoParent_DeleteSourceKeepsConflictedChild asserts a conflicted merge
// keeps the child regardless of deleteSource: the user still has to resolve it.
func TestMergeIntoParent_DeleteSourceKeepsConflictedChild(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ConflictsPending: true}}
	r, h := newFoldRouter(reader, hierarchy, &fakeLastErrors{})

	rec := mergeInto(r, `{"strategy":"merge","deleteSource":true}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Empty(t, hierarchy.gotDeleteID, "a conflicted merge leaves the child to resolve")
}

// TestMergeIntoParent_DeleteSourceWithoutFlagKeepsChild asserts the fold is opt-in:
// a clean merge without deleteSource leaves the child alone.
func TestMergeIntoParent_DeleteSourceWithoutFlagKeepsChild(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ParentTipSha: "abc123"}}
	r, h := newFoldRouter(reader, hierarchy, &fakeLastErrors{})

	rec := mergeInto(r, `{"strategy":"squash"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Empty(t, hierarchy.gotDeleteID)
}

// TestMergeIntoParent_FoldLeafLookupFailureSurfacesLastError asserts a post-merge
// cleanup failure is reported as such: the MERGE already succeeded, so the message
// must not read as a failed merge.
func TestMergeIntoParent_FoldLeafLookupFailureSurfacesLastError(
	t *testing.T,
) {
	reader := &fakeReader{
		get:     domain.Workspace{ID: "child"},
		listErr: errors.New("the workspace index is unreadable"),
	}
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ParentTipSha: "abc123"}}
	lastErrors := &fakeLastErrors{}
	r, h := newFoldRouter(reader, hierarchy, lastErrors)

	rec := mergeInto(r, `{"strategy":"squash","deleteSource":true}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Empty(t, hierarchy.gotDeleteID, "an unknown leaf state must not delete anything")
	assert.Equal(t, "child", lastErrors.gotID)
	assert.Contains(t, lastErrors.gotMsg, "merge succeeded but post-merge cleanup failed")
}

// TestMergeIntoParent_FoldDeleteFailureSurfacesLastError asserts the same for the
// delete itself failing after a successful merge.
func TestMergeIntoParent_FoldDeleteFailureSurfacesLastError(
	t *testing.T,
) {
	reader := &fakeReader{
		get:  domain.Workspace{ID: "child"},
		list: []domain.Workspace{{ID: "child", ParentID: "parent"}},
	}
	hierarchy := &fakeHierarchy{
		mergeResult: worktree.MergeResult{ParentTipSha: "abc123"},
		deleteErr:   errors.New("the worktree is locked"),
	}
	lastErrors := &fakeLastErrors{}
	r, h := newFoldRouter(reader, hierarchy, lastErrors)

	rec := mergeInto(r, `{"strategy":"squash","deleteSource":true}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Equal(t, "child", lastErrors.gotID)
	assert.Contains(t, lastErrors.gotMsg, "merge succeeded but removing the workspace failed")
}

// TestRebaseOntoParent_Returns202 asserts the fail-fast/good-path-async contract
// for the user-initiated "finish the move": validation (the workspace exists) runs
// synchronously, then 202 and the rebase runs in the background.
func TestRebaseOntoParent_Returns202(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{}
	r, h := newFoldRouter(reader, hierarchy, &fakeLastErrors{})

	rec := do(
		r,
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/rebase-onto-parent",
		"",
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	h.WaitAsync()
	assert.Equal(t, "child", hierarchy.gotRebaseID)
}

// TestRebaseOntoParentMissingWorkspace_4xx asserts the synchronous existence check
// rejects an unknown workspace before any 202 or background rebase.
func TestRebaseOntoParentMissingWorkspace_4xx(
	t *testing.T,
) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	hierarchy := &fakeHierarchy{}
	r, h := newFoldRouter(reader, hierarchy, &fakeLastErrors{})

	rec := do(
		r,
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/nope/rebase-onto-parent",
		"",
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	h.WaitAsync()
	assert.Empty(t, hierarchy.gotRebaseID, "no rebase runs for a workspace that does not exist")
}

// TestRebaseOntoParentAsyncErrorBroadcastsLastError asserts a background rebase
// failure surfaces on the entity, not on the HTTP response (already a 202).
func TestRebaseOntoParentAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	hierarchy := &fakeHierarchy{rebaseErr: errors.New("rebase refused: the parent moved")}
	lastErrors := &fakeLastErrors{}
	r, h := newFoldRouter(reader, hierarchy, lastErrors)

	rec := do(
		r,
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/rebase-onto-parent",
		"",
	)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Equal(t, "child", lastErrors.gotID)
	assert.Contains(t, lastErrors.gotMsg, "rebase refused")
}
