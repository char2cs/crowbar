package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

const chatBase = "/v0/projects/p1/repos/r1/chats/c1"

func heldWorkspace() *fakeWorktrees {
	return &fakeWorktrees{ws: domain.Workspace{ID: "w1"}}
}

func TestChatLock_LocksTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{}
	worktrees := heldWorkspace()
	r, _ := newChatRouter(reader, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/lock", `{"locked":true}`)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "c1", worktrees.gotChat, "the chat in the URL is what gets resolved")
	require.Equal(t, 1, reader.lockCalls)
	require.NotNil(t, reader.gotLocked)
	assert.True(t, *reader.gotLocked)
}

func TestChatLock_OmittedLockedStillHandsTheQuestionBackToTheProvider(
	t *testing.T,
) {
	reader := &fakeReader{}
	r, _ := newChatRouter(reader, &fakeHierarchy{}, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/lock", `{}`)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 1, reader.lockCalls)
	assert.Nil(t, reader.gotLocked)
}

func TestChatLock_RejectsAMalformedBodyBeforeResolvingAnything(
	t *testing.T,
) {
	reader := &fakeReader{}
	worktrees := heldWorkspace()
	r, _ := newChatRouter(reader, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/lock", `{"locked":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, reader.lockCalls, "a malformed body must never reach the store")
	assert.Zero(t, worktrees.calls, "nor cost a resolve")
}

func TestChatSync_SyncsTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{syncDone: make(chan struct{})}
	r, _ := newChatRouter(reader, &fakeHierarchy{}, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/sync", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, reader.syncDone)
	assert.Equal(t, "w1", reader.gotSync)
}

func TestChatMergeIntoParent_MergesTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{
		mergeResult: workspace.MergeResult{ParentTipSha: "abc123"},
		mergeDone:   make(chan struct{}),
	}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"squash"}`)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.mergeDone)
	assert.Equal(t, "w1", hierarchy.gotMergeID)
	assert.Equal(t, gitdomain.MergeStrategySquash, hierarchy.gotStrategy)
}

func TestChatMergeIntoParent_StillRefusesAMissingStrategy(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotMergeID)
}

func TestChatMergeIntoParent_StillFoldsAMergedLeafAway(
	t *testing.T,
) {
	reader := &fakeReader{list: []domain.Workspace{{ID: "w1"}}}
	hierarchy := &fakeHierarchy{deleteDone: make(chan struct{})}
	r, _ := newChatRouter(reader, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent",
		`{"strategy":"squash","deleteSource":true}`)

	require.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.deleteDone)
	assert.Equal(t, "w1", hierarchy.gotDeleteID)
}

func TestChatReparent_ReparentsTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparentDone: make(chan struct{})}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/reparent", `{"newParentId":"w9"}`)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.reparentDone)
	assert.Equal(t, "w1", hierarchy.gotReparent)
	assert.Equal(t, "w9", hierarchy.gotNewParent)
}

func TestChatReparent_StillRefusesAMissingNewParent(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/reparent", `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotReparent)
}

func TestChatRebaseOntoParent_RebasesTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{rebaseDone: make(chan struct{})}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.rebaseDone)
	assert.Equal(t, "w1", hierarchy.gotRebaseID)
}

func TestChatRetryProvision_ReprovisionsTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{retryDone: make(chan struct{})}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.retryDone)
	assert.Equal(t, "w1", hierarchy.gotRetryID)
}

func TestChatDetachHolder_DetachesTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{detachDone: make(chan struct{})}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/detach-holder", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.detachDone)
	assert.Equal(t, "w1", hierarchy.gotDetachID)
}

func TestChatVerbs_ActOnTheAncestorsWorktreeAChatShares(
	t *testing.T,
) {
	worktrees := &fakeWorktrees{ws: domain.Workspace{ID: "ancestor-ws"}}
	hierarchy := &fakeHierarchy{retryDone: make(chan struct{})}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")

	require.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.retryDone)
	assert.Equal(t, "ancestor-ws", hierarchy.gotRetryID)
}

func TestChatVerbs_404WhenNoAncestorOwnsAWorktree(
	t *testing.T,
) {
	for _, verb := range []string{
		"lock", "sync", "merge-into-parent", "reparent",
		"rebase-onto-parent", "retry-provision", "detach-holder",
	} {
		t.Run(verb, func(t *testing.T) {
			reader := &fakeReader{}
			hierarchy := &fakeHierarchy{}
			worktrees := &fakeWorktrees{err: worktree.ErrNoWorktreeInAncestry}
			r, ops := newChatRouter(reader, hierarchy, worktrees)

			rec := do(r, http.MethodPost, chatBase+"/"+verb,
				`{"locked":true,"strategy":"squash","newParentId":"w9"}`)

			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			assert.Empty(t, ops.frames, "no operation starts for a chat holding nothing")
			assert.Zero(t, reader.lockCalls)
			assert.Empty(t, reader.gotSync)
			assert.Empty(t, hierarchy.gotMergeID)
			assert.Empty(t, hierarchy.gotReparent)
			assert.Empty(t, hierarchy.gotRebaseID)
			assert.Empty(t, hierarchy.gotRetryID)
			assert.Empty(t, hierarchy.gotDetachID)
		})
	}
}

func TestChatVerbs_404WhenTheResolveFailsForAnyOtherReason(
	t *testing.T,
) {
	worktrees := &fakeWorktrees{err: errors.New("the read model is down")}
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestChatRenameBranch_RenamesTheBranchOfTheWorktreeTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{renamed: domain.Workspace{ID: "w1", Branch: "feature/x"}}
	worktrees := heldWorkspace()
	r, _ := newChatRouter(&fakeReader{}, hierarchy, worktrees)

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "c1", worktrees.gotChat, "the chat in the URL is what gets resolved")
	assert.Equal(t, "w1", hierarchy.gotRenameID,
		"and the workspace it holds is what gets renamed")
	assert.Equal(t, "feature/x", hierarchy.gotRenameTo)
	assert.Contains(t, rec.Body.String(), "c1",
		"the answer names the CHAT — past law 1 a workspace has no id a client may hold")
}

func TestChatRenameBranch_CarriesEveryGuardTheUsecaseEnforces(
	t *testing.T,
) {
	for name, refusal := range map[string]error{
		"locked branch":             workspace.ErrWorkspaceLocked,
		"branch name taken":         workspace.ErrBranchWorkspaceExists,
		"unprovisioned placeholder": workspace.ErrParentUnprovisioned,
		"adopted checkout":          workspace.ErrRenameUnmanagedWorkspace,
	} {
		t.Run(name, func(t *testing.T) {
			r, _ := newChatRouter(
				&fakeReader{}, &fakeHierarchy{renameErr: refusal}, heldWorkspace())

			rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`)

			assert.Equal(t, http.StatusConflict, rec.Code)
			assert.Contains(t, rec.Body.String(), refusal.Error())
		})
	}
}

func TestChatRenameBranch_BlankBranchIsRefusedBeforeTheUsecase(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newChatRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"   "}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotRenameID, "a blank name must never reach the usecase")
}

func TestChatRenameBranch_BadJSONIsRefusedBeforeTheChatIsResolved(
	t *testing.T,
) {
	worktrees := heldWorkspace()
	r, _ := newChatRouter(&fakeReader{}, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, worktrees.gotChat, "nothing was resolved for an unparseable request")
}

func TestChatRenameBranch_UnresolvableChatIs404(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newChatRouter(
		&fakeReader{}, hierarchy, &fakeWorktrees{err: worktree.ErrNoWorktreeInAncestry})

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotRenameID, "nothing may be renamed for a chat holding nothing")
}
