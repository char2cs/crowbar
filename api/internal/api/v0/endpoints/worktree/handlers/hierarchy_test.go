package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// childChat is the chat these tests address, resolving to the workspace named
// "child" throughout.
func childChat() *fakeWorktrees {
	return &fakeWorktrees{ws: domain.Workspace{ID: "child"}}
}

// TestMergeIntoParent_Returns202 asserts the fail-fast/good-path-async contract:
// a valid merge-into-parent passes synchronous validation (body shape, the chat
// resolves), returns 202 with an empty body, and runs MergeIntoParent in the
// background. The merge outcome is delivered on the WebSocket stream (blackbox in
// W13), not in the HTTP response.
func TestMergeIntoParent_Returns202(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{
		mergeResult: workspace.MergeResult{ParentTipSha: "abc123"},
		mergeDone:   make(chan struct{}),
	}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, childChat())
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"squash"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.mergeDone)
	assert.Equal(t, "child", hierarchy.gotMergeID)
	assert.Equal(t, gitdomain.MergeStrategySquash, hierarchy.gotStrategy)
}

func TestMergeIntoParentBadJSON(
	t *testing.T,
) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{not-json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMergeIntoParentMissingStrategy(
	t *testing.T,
) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestMergeIntoParentMissingWorkspace_4xx asserts the synchronous existence
// check: a merge for a chat holding no worktree is rejected on the request path
// with a 4xx before any 202 or background merge.
func TestMergeIntoParentMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, &fakeWorktrees{err: apperr.ErrNotFound})
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"merge"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotMergeID, "merge must not run when the workspace is missing")
}

// TestMergeIntoParentAsyncErrorBroadcastsLastError asserts a background merge
// failure surfaces on the workspace entity via SetLastError, not on the HTTP
// response (the handler already returned 202).
func TestMergeIntoParentAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeErr: workspace.ErrParentLocked}
	lastErrors := &fakeLastErrors{called: make(chan struct{}, 1)}
	r, _ := newChatRouterWithErrors(&fakeReader{}, hierarchy, childChat(), lastErrors)
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"merge"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	// The SetLastError call IS the signal that the failed merge surfaced on the
	// entity; block on it rather than guessing at a duration.
	<-lastErrors.called
	assert.Equal(t, "child", lastErrors.gotID)
}

// TestReparent_Returns202 asserts the fail-fast/good-path-async contract: a valid
// reparent passes synchronous validation (body shape, the chat resolves), returns
// 202 with an empty body, and runs Reparent in the background. The reparented
// workspace is delivered on the WebSocket stream (blackbox in W13), not in the
// HTTP response.
func TestReparent_Returns202(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{
		reparented:   domain.Workspace{ID: "child", ParentID: "np"},
		reparentDone: make(chan struct{}),
	}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, childChat())
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"np"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, hierarchy.reparentDone)
	assert.Equal(t, "child", hierarchy.gotReparent)
	assert.Equal(t, "np", hierarchy.gotNewParent)
}

func TestReparentBadJSON(
	t *testing.T,
) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{not-json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReparentMissingNewParent(
	t *testing.T,
) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestReparentMissingWorkspace_4xx asserts the synchronous existence check: a
// reparent for a chat holding no worktree is rejected on the request path with a
// 4xx before any 202 or background reparent.
func TestReparentMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, &fakeWorktrees{err: apperr.ErrNotFound})
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"np"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotReparent, "reparent must not run when the workspace is missing")
}

// TestReparentAsyncErrorBroadcastsLastError asserts a background reparent failure
// surfaces on the workspace entity via SetLastError, not on the HTTP response.
func TestReparentAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparentErr: workspace.ErrChildHasChildren}
	lastErrors := &fakeLastErrors{called: make(chan struct{}, 1)}
	r, _ := newChatRouterWithErrors(&fakeReader{}, hierarchy, childChat(), lastErrors)
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"np"}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	// The SetLastError call IS the signal that the failed reparent surfaced on
	// the entity; block on it rather than guessing at a duration.
	<-lastErrors.called
	assert.Equal(t, "child", lastErrors.gotID)
}

func TestRetryProvision_Returns202(t *testing.T) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestDetachHolder_Returns202(t *testing.T) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/detach-holder", "")
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestRetryProvisionMissingWorkspace_4xx(t *testing.T) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, &fakeWorktrees{err: apperr.ErrNotFound})
	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func mergeInto(
	r *gin.Engine,
	body string,
) *httptest.ResponseRecorder {
	return do(r, http.MethodPost, chatBase+"/merge-into-parent", body)
}

// TestMergeIntoParent_DeleteSourceFoldsMergedLeaf asserts the fold: a clean merge
// with deleteSource removes the child once it is a LEAF, because at that point its
// work lives in the parent and nothing else descends from it.
func TestMergeIntoParent_DeleteSourceFoldsMergedLeaf(
	t *testing.T,
) {
	reader := &fakeReader{
		// Siblings under the same parent do not make "child" a non-leaf: only a
		// workspace whose ParentID IS "child" would.
		list: []domain.Workspace{
			{ID: "child", ParentID: "parent"},
			{ID: "sibling", ParentID: "parent"},
		},
	}
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	r, h := newChatRouter(reader, hierarchy, childChat())

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
		list: []domain.Workspace{
			{ID: "child", ParentID: "parent"},
			{ID: "grandchild", ParentID: "child"},
		},
	}
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	lastErrors := &fakeLastErrors{}
	r, h := newChatRouterWithErrors(reader, hierarchy, childChat(), lastErrors)

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
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ConflictsPending: true}}
	r, h := newChatRouter(&fakeReader{}, hierarchy, childChat())

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
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	r, h := newChatRouter(&fakeReader{}, hierarchy, childChat())

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
	reader := &fakeReader{listErr: errors.New("the workspace index is unreadable")}
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	lastErrors := &fakeLastErrors{}
	r, h := newChatRouterWithErrors(reader, hierarchy, childChat(), lastErrors)

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
	reader := &fakeReader{list: []domain.Workspace{{ID: "child", ParentID: "parent"}}}
	hierarchy := &fakeHierarchy{
		mergeResult: workspace.MergeResult{ParentTipSha: "abc123"},
		deleteErr:   errors.New("the worktree is locked"),
	}
	lastErrors := &fakeLastErrors{}
	r, h := newChatRouterWithErrors(reader, hierarchy, childChat(), lastErrors)

	rec := mergeInto(r, `{"strategy":"squash","deleteSource":true}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Equal(t, "child", lastErrors.gotID)
	assert.Contains(t, lastErrors.gotMsg, "merge succeeded but removing the workspace failed")
}

// TestRebaseOntoParent_Returns202 asserts the fail-fast/good-path-async contract
// for the user-initiated "finish the move": validation (the chat resolves) runs
// synchronously, then 202 and the rebase runs in the background.
func TestRebaseOntoParent_Returns202(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, h := newChatRouter(&fakeReader{}, hierarchy, childChat())

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	h.WaitAsync()
	assert.Equal(t, "child", hierarchy.gotRebaseID)
}

// TestRebaseOntoParentMissingWorkspace_4xx asserts the synchronous existence check
// rejects a chat holding no worktree before any 202 or background rebase.
func TestRebaseOntoParentMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, h := newChatRouter(&fakeReader{}, hierarchy, &fakeWorktrees{err: apperr.ErrNotFound})

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	h.WaitAsync()
	assert.Empty(t, hierarchy.gotRebaseID, "no rebase runs for a workspace that does not exist")
}

// TestRebaseOntoParentAsyncErrorBroadcastsLastError asserts a background rebase
// failure surfaces on the entity, not on the HTTP response (already a 202).
func TestRebaseOntoParentAsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{rebaseErr: errors.New("rebase refused: the parent moved")}
	lastErrors := &fakeLastErrors{}
	r, h := newChatRouterWithErrors(&fakeReader{}, hierarchy, childChat(), lastErrors)

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	assert.Equal(t, http.StatusAccepted, rec.Code)
	h.WaitAsync()
	assert.Equal(t, "child", lastErrors.gotID)
	assert.Contains(t, lastErrors.gotMsg, "rebase refused")
}
