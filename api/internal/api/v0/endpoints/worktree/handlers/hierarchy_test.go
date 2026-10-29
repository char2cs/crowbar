package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// childChat is the chat these tests address, resolving to the workspace named
// "child" throughout.
func childChat() *fakeWorktrees {
	return &fakeWorktrees{ws: domain.Workspace{ID: "child", ProjectID: "p1", Branch: "feature/child", ParentID: "parent"}}
}

// family is the workspace list the labels name the parent from.
func family() *fakeReader {
	return &fakeReader{list: []domain.Workspace{
		{ID: "parent", Branch: "main"},
		{ID: "child", ParentID: "parent", Branch: "feature/child"},
		{ID: "np", Branch: "feature/np"},
	}}
}

// A valid merge-into-parent answers 202 with its operation, which runs the merge.
func TestMergeIntoParent_AnswersItsOperation(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	r, ops := newChatRouter(family(), hierarchy, childChat())
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"squash"}`)

	op := accepted(t, rec)
	assert.Equal(t, domain.OpWorkspaceMergeIntoParent, op.Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityWorkspace, ID: "child"}, op.Subject)
	assert.Equal(t, "p1", op.ProjectID)
	assert.Equal(t, "Merge feature/child into main", op.Label)
	assert.Equal(t, domain.OpSucceeded, ops.settled(t, rec).State)
	assert.Equal(t, "child", hierarchy.gotMergeID)
	assert.Equal(t, gitdomain.MergeStrategySquash, hierarchy.gotStrategy)
}

// A parent that cannot be named leaves the label at the verb and branch.
func TestMergeIntoParent_AnUnknownParentIsLeftOutOfTheLabel(t *testing.T) {
	r, ops := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"squash"}`)
	assert.Equal(t, "Merge feature/child", accepted(t, rec).Label)
	ops.settled(t, rec)
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

// A merge for a chat holding no worktree is refused on the request path, and
// starts no operation.
func TestMergeIntoParentMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, &fakeWorktrees{err: problem.ErrNotFound})
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"merge"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, ops.frames, "no operation starts for a workspace that does not exist")
}

// A merge that fails after its 202 fails its operation with the reason.
func TestMergeIntoParent_AFailureFailsTheOperation(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeErr: workspace.ErrParentLocked}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, childChat())
	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"merge"}`)

	op := ops.settled(t, rec)
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Equal(t, workspace.ErrParentLocked.Error(), op.Error.Message)
}

// A valid reparent answers 202 with its operation, which runs the reparent.
func TestReparent_AnswersItsOperation(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparented: domain.Workspace{ID: "child", ParentID: "np"}}
	r, ops := newChatRouter(family(), hierarchy, childChat())
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"np"}`)

	op := accepted(t, rec)
	assert.Equal(t, domain.OpWorkspaceReparent, op.Kind)
	assert.Equal(t, "Move feature/child under feature/np", op.Label)
	assert.Equal(t, domain.OpSucceeded, ops.settled(t, rec).State)
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

// A reparent for a chat holding no worktree is refused on the request path.
func TestReparentMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, &fakeWorktrees{err: problem.ErrNotFound})
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"np"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, ops.frames, "no operation starts for a workspace that does not exist")
}

// A refused reparent fails its operation with the refusal's code.
func TestReparent_ARefusalFailsTheOperation(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparentErr: workspace.ErrChildHasChildren}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, childChat())
	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"np"}`)

	op := ops.settled(t, rec)
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Equal(t, problem.Conflict, op.Error.Code)
}

func TestRetryProvision_AnswersItsOperation(t *testing.T) {
	r, ops := newChatRouter(&fakeReader{}, &fakeHierarchy{}, childChat())
	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")
	op := accepted(t, rec)
	assert.Equal(t, domain.OpWorkspaceRetryProvision, op.Kind)
	assert.Equal(t, "Retry feature/child", op.Label)
	assert.Equal(t, domain.OpSucceeded, ops.settled(t, rec).State)
}

func TestDetachHolder_AnswersItsOperation(t *testing.T) {
	worktrees := &fakeWorktrees{ws: domain.Workspace{ID: "child", HeldByPath: "/repo"}}
	r, ops := newChatRouter(&fakeReader{}, &fakeHierarchy{}, worktrees)
	rec := do(r, http.MethodPost, chatBase+"/detach-holder", "")
	op := accepted(t, rec)
	assert.Equal(t, domain.OpWorkspaceDetachHolder, op.Kind)
	assert.Equal(t, "Detach /repo", op.Label)
	assert.Equal(t, domain.OpSucceeded, ops.settled(t, rec).State)
}

func TestRetryProvisionMissingWorkspace_4xx(t *testing.T) {
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, &fakeWorktrees{err: problem.ErrNotFound})
	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func mergeInto(
	r *gin.Engine,
	body string,
) *httptest.ResponseRecorder {
	return do(r, http.MethodPost, chatBase+"/merge-into-parent", body)
}

// A clean merge with deleteSource removes the child once it is a LEAF, because
// at that point its work lives in the parent and nothing else descends from it.
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
	r, ops := newChatRouter(reader, hierarchy, childChat())

	op := ops.settled(t, mergeInto(r, `{"strategy":"squash","deleteSource":true}`))

	assert.Equal(t, domain.OpSucceeded, op.State)
	assert.Equal(t, "child", hierarchy.gotDeleteID, "a merged leaf is folded away")
}

// A merged child that still has descendants is KEPT, because DeleteCascade
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
	r, ops := newChatRouter(reader, hierarchy, childChat())

	op := ops.settled(t, mergeInto(r, `{"strategy":"squash","deleteSource":true}`))

	assert.Empty(t, hierarchy.gotDeleteID, "a child with descendants must survive the merge")
	assert.Equal(t, domain.OpSucceeded, op.State, "keeping a non-leaf is a no-op, not a failure")
}

// A conflicted merge keeps the child regardless of deleteSource.
func TestMergeIntoParent_DeleteSourceKeepsConflictedChild(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ConflictsPending: true}}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, childChat())

	ops.settled(t, mergeInto(r, `{"strategy":"merge","deleteSource":true}`))

	assert.Empty(t, hierarchy.gotDeleteID, "a conflicted merge leaves the child to resolve")
}

// The fold is opt-in: a clean merge without deleteSource leaves the child alone.
func TestMergeIntoParent_DeleteSourceWithoutFlagKeepsChild(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, childChat())

	ops.settled(t, mergeInto(r, `{"strategy":"squash"}`))

	assert.Empty(t, hierarchy.gotDeleteID)
}

// A post-merge cleanup failure is reported as such: the MERGE already
// succeeded, so the message must not read as a failed merge.
func TestMergeIntoParent_FoldLeafLookupFailureFailsTheOperation(
	t *testing.T,
) {
	reader := &fakeReader{listErr: errors.New("the workspace index is unreadable")}
	hierarchy := &fakeHierarchy{mergeResult: workspace.MergeResult{ParentTipSha: "abc123"}}
	r, ops := newChatRouter(reader, hierarchy, childChat())

	op := ops.settled(t, mergeInto(r, `{"strategy":"squash","deleteSource":true}`))

	assert.Empty(t, hierarchy.gotDeleteID, "an unknown leaf state must not delete anything")
	require.NotNil(t, op.Error)
	assert.Contains(t, op.Error.Message, "merge succeeded but post-merge cleanup failed")
}

// The same for the delete itself failing after a successful merge.
func TestMergeIntoParent_FoldDeleteFailureFailsTheOperation(
	t *testing.T,
) {
	reader := &fakeReader{list: []domain.Workspace{{ID: "child", ParentID: "parent"}}}
	hierarchy := &fakeHierarchy{
		mergeResult: workspace.MergeResult{ParentTipSha: "abc123"},
		deleteErr:   errors.New("the worktree is locked"),
	}
	r, ops := newChatRouter(reader, hierarchy, childChat())

	op := ops.settled(t, mergeInto(r, `{"strategy":"squash","deleteSource":true}`))

	require.NotNil(t, op.Error)
	assert.Contains(t, op.Error.Message, "merge succeeded but removing the workspace failed")
}

// The user-initiated "finish the move" answers 202 with its operation.
func TestRebaseOntoParent_AnswersItsOperation(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, ops := newChatRouter(family(), hierarchy, childChat())

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	op := accepted(t, rec)
	assert.Equal(t, domain.OpWorkspaceRebaseOntoParent, op.Kind)
	assert.Equal(t, "Rebase feature/child onto main", op.Label)
	ops.settled(t, rec)
	assert.Equal(t, "child", hierarchy.gotRebaseID)
}

// A chat holding no worktree starts no rebase.
func TestRebaseOntoParentMissingWorkspace_4xx(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, &fakeWorktrees{err: problem.ErrNotFound})

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, ops.frames, "no rebase runs for a workspace that does not exist")
}

func TestRebaseOntoParent_AFailureFailsTheOperation(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{rebaseErr: errors.New("rebase refused: the parent moved")}
	r, ops := newChatRouter(&fakeReader{}, hierarchy, childChat())

	op := ops.settled(t, do(r, http.MethodPost, chatBase+"/rebase-onto-parent", ""))

	require.NotNil(t, op.Error)
	assert.Contains(t, op.Error.Message, "rebase refused")
}
