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

// The seven worktree lifecycle verbs, addressed by the CHAT that holds the
// worktree instead of by the worktree (spec §4.3).
//
// Every test here asks one question: does the chat-keyed route reach the same
// usecase call, with the same argument, that the :wsId route reaches? Not "does
// it work" — the :wsId routes are already covered — but "is it the same verb".
// That is why most of these fire BOTH routes against ONE handler value and
// compare, rather than asserting a hard-coded expectation twice.

const (
	chatBase = "/v0/projects/p1/repos/r1/chats/c1"
	wsBase   = "/v0/projects/p1/repos/r1/workspaces/w1"
)

// heldWorkspace is what the resolver answers for chat c1 throughout: the
// workspace behind the worktree that chat reads and writes through.
func heldWorkspace() *fakeWorktrees {
	return &fakeWorktrees{ws: domain.Workspace{ID: "w1"}}
}

func TestChatLock_LocksTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	worktrees := heldWorkspace()
	r, _ := newPairedRouter(reader, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/lock", `{"locked":true}`)

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "c1", worktrees.gotChat, "the chat in the URL is what gets resolved")
	require.Equal(t, 1, reader.lockCalls)
	require.NotNil(t, reader.gotLocked)
	assert.True(t, *reader.gotLocked)
}

// The lock override's third answer survives the re-keying: an omitted `locked`
// still clears the override rather than reading as false.
func TestChatLock_OmittedLockedStillHandsTheQuestionBackToTheProvider(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	r, _ := newPairedRouter(reader, &fakeHierarchy{}, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/lock", `{}`)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 1, reader.lockCalls)
	assert.Nil(t, reader.gotLocked)
}

// A malformed body is refused BEFORE the chat is resolved, exactly as the
// :wsId route refuses it before the workspace is read: a body that cannot be
// parsed is not a request about any particular chat yet.
func TestChatLock_RejectsAMalformedBodyBeforeResolvingAnything(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	worktrees := heldWorkspace()
	r, _ := newPairedRouter(reader, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/lock", `{"locked":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, reader.lockCalls, "a malformed body must never reach the store")
	assert.Zero(t, worktrees.calls, "nor cost a resolve")
}

func TestChatSync_SyncsTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}, syncDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, &fakeHierarchy{}, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/sync", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, reader.syncDone)
	assert.Equal(t, "w1", reader.gotSync)
}

func TestChatMergeIntoParent_MergesTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	hierarchy := &fakeHierarchy{
		mergeResult: workspace.MergeResult{ParentTipSha: "abc123"},
		mergeDone:   make(chan struct{}),
	}
	r, _ := newPairedRouter(reader, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"squash"}`)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.mergeDone)
	assert.Equal(t, "w1", hierarchy.gotMergeID)
	assert.Equal(t, gitdomain.MergeStrategySquash, hierarchy.gotStrategy)
}

// The synchronous guards travel with the verb: a merge with no strategy is
// still refused on the request path, and still reaches no git.
func TestChatMergeIntoParent_StillRefusesAMissingStrategy(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newPairedRouter(&fakeReader{get: domain.Workspace{ID: "w1"}}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent", `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotMergeID)
}

// deleteSource's post-merge fold is a side effect of the verb, not of the
// route: a clean merge of a LEAF still cascades the child away when asked.
func TestChatMergeIntoParent_StillFoldsAMergedLeafAway(
	t *testing.T,
) {
	// One workspace in the list and nothing parented under w1 — a leaf.
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}, list: []domain.Workspace{{ID: "w1"}}}
	hierarchy := &fakeHierarchy{deleteDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/merge-into-parent",
		`{"strategy":"squash","deleteSource":true}`)

	require.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.deleteDone)
	assert.Equal(t, "w1", hierarchy.gotDeleteID)
}

func TestChatReparent_ReparentsTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	hierarchy := &fakeHierarchy{reparentDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, hierarchy, heldWorkspace())

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
	r, _ := newPairedRouter(&fakeReader{get: domain.Workspace{ID: "w1"}}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/reparent", `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotReparent)
}

func TestChatRebaseOntoParent_RebasesTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	hierarchy := &fakeHierarchy{rebaseDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/rebase-onto-parent", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.rebaseDone)
	assert.Equal(t, "w1", hierarchy.gotRebaseID)
}

func TestChatRetryProvision_ReprovisionsTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	hierarchy := &fakeHierarchy{retryDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.retryDone)
	assert.Equal(t, "w1", hierarchy.gotRetryID)
}

func TestChatDetachHolder_DetachesTheWorkspaceTheChatHolds(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
	hierarchy := &fakeHierarchy{detachDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPost, chatBase+"/detach-holder", "")

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	waitClosed(t, hierarchy.detachDone)
	assert.Equal(t, "w1", hierarchy.gotDetachID)
}

// A chat that resolves to a workspace SHARED with its ancestors is served by
// the workspace it resolves to, not by one it happens to own itself: a thread
// hanging under a worktree-owning chat locks THAT worktree.
func TestChatVerbs_ActOnTheAncestorsWorktreeAChatShares(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "ancestor-ws"}}
	// The resolver's whole job: c1 owns nothing, its ancestor owns ancestor-ws.
	worktrees := &fakeWorktrees{ws: domain.Workspace{ID: "ancestor-ws"}}
	hierarchy := &fakeHierarchy{retryDone: make(chan struct{})}
	r, _ := newPairedRouter(reader, hierarchy, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")

	require.Equal(t, http.StatusAccepted, rec.Code)
	waitClosed(t, hierarchy.retryDone)
	assert.Equal(t, "ancestor-ws", hierarchy.gotRetryID)
}

// A bubble hanging off nothing has no worktree for these verbs to act on. It is
// answered 404 — the same answer the flat /chats/:chatId group's own middleware
// gives — and never a panic, a 500, or a call keyed on an empty id.
func TestChatVerbs_404WhenNoAncestorOwnsAWorktree(
	t *testing.T,
) {
	for _, verb := range []string{
		"lock", "sync", "merge-into-parent", "reparent",
		"rebase-onto-parent", "retry-provision", "detach-holder",
	} {
		t.Run(verb, func(t *testing.T) {
			reader := &fakeReader{get: domain.Workspace{ID: "w1"}}
			hierarchy := &fakeHierarchy{}
			worktrees := &fakeWorktrees{err: worktree.ErrNoWorktreeInAncestry}
			r, h := newPairedRouter(reader, hierarchy, worktrees)

			// Bodies that would otherwise pass their own validation, so the 404 is
			// provably the resolve and not a body guard firing first.
			rec := do(r, http.MethodPost, chatBase+"/"+verb,
				`{"locked":true,"strategy":"squash","newParentId":"w9"}`)

			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			h.WaitAsync()
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

// Any other resolve failure answers the same 404 for the same reason: from the
// caller's side a chat whose worktree cannot be resolved is indistinguishable
// from a chat that does not exist.
func TestChatVerbs_404WhenTheResolveFailsForAnyOtherReason(
	t *testing.T,
) {
	worktrees := &fakeWorktrees{err: errors.New("the read model is down")}
	r, _ := newPairedRouter(&fakeReader{}, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPost, chatBase+"/retry-provision", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// The equivalence, asserted directly: the same operation reached both ways
// lands the same call on the same usecase, and the ONLY difference between the
// two requests is how they named the target.
func TestChatVerbs_ReachTheSameUsecaseCallAsTheirWorkspaceKeyedTwin(
	t *testing.T,
) {
	t.Run("lock", func(t *testing.T) {
		viaChat := &fakeReader{get: domain.Workspace{ID: "w1"}}
		rc, _ := newPairedRouter(viaChat, &fakeHierarchy{}, heldWorkspace())
		chatRec := do(rc, http.MethodPost, chatBase+"/lock", `{"locked":false}`)

		viaWS := &fakeReader{get: domain.Workspace{ID: "w1"}}
		rw, _ := newPairedRouter(viaWS, &fakeHierarchy{}, heldWorkspace())
		wsRec := do(rw, http.MethodPost, wsBase+"/lock", `{"locked":false}`)

		assert.Equal(t, wsRec.Code, chatRec.Code)
		assert.Equal(t, viaWS.lockCalls, viaChat.lockCalls)
		require.NotNil(t, viaChat.gotLocked)
		require.NotNil(t, viaWS.gotLocked)
		assert.Equal(t, *viaWS.gotLocked, *viaChat.gotLocked)
	})

	t.Run("merge-into-parent", func(t *testing.T) {
		viaChat := &fakeHierarchy{mergeDone: make(chan struct{})}
		rc, _ := newPairedRouter(&fakeReader{get: domain.Workspace{ID: "w1"}}, viaChat, heldWorkspace())
		chatRec := do(rc, http.MethodPost, chatBase+"/merge-into-parent", `{"strategy":"rebase"}`)
		waitClosed(t, viaChat.mergeDone)

		viaWS := &fakeHierarchy{mergeDone: make(chan struct{})}
		rw, _ := newPairedRouter(&fakeReader{get: domain.Workspace{ID: "w1"}}, viaWS, heldWorkspace())
		wsRec := do(rw, http.MethodPost, wsBase+"/merge-into-parent", `{"strategy":"rebase"}`)
		waitClosed(t, viaWS.mergeDone)

		assert.Equal(t, wsRec.Code, chatRec.Code)
		assert.Equal(t, viaWS.gotMergeID, viaChat.gotMergeID)
		assert.Equal(t, viaWS.gotStrategy, viaChat.gotStrategy)
	})

	t.Run("reparent", func(t *testing.T) {
		viaChat := &fakeHierarchy{reparentDone: make(chan struct{})}
		rc, _ := newPairedRouter(&fakeReader{get: domain.Workspace{ID: "w1"}}, viaChat, heldWorkspace())
		chatRec := do(rc, http.MethodPost, chatBase+"/reparent", `{"newParentId":"w9"}`)
		waitClosed(t, viaChat.reparentDone)

		viaWS := &fakeHierarchy{reparentDone: make(chan struct{})}
		rw, _ := newPairedRouter(&fakeReader{get: domain.Workspace{ID: "w1"}}, viaWS, heldWorkspace())
		wsRec := do(rw, http.MethodPost, wsBase+"/reparent", `{"newParentId":"w9"}`)
		waitClosed(t, viaWS.reparentDone)

		assert.Equal(t, wsRec.Code, chatRec.Code)
		assert.Equal(t, viaWS.gotReparent, viaChat.gotReparent)
		assert.Equal(t, viaWS.gotNewParent, viaChat.gotNewParent)
	})
}

// The chat-keyed BRANCH rename (spec §5's missing half). Every test below asks
// the same question the rest of this file does — is it the SAME verb? — because
// the whole reason this route exists rather than pointing a client at the raw
// PATCH /v0/chats/:chatId/git/branches is that the raw one enforces none of
// these guards and leaves domain.Workspace.Branch stale behind a bare
// `git branch -m`.

func TestChatRenameBranch_RenamesTheBranchOfTheWorktreeTheChatHolds(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{renamed: domain.Workspace{ID: "w1", Branch: "feature/x"}}
	worktrees := heldWorkspace()
	r, _ := newPairedRouter(&fakeReader{}, hierarchy, worktrees)

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "c1", worktrees.gotChat, "the chat in the URL is what gets resolved")
	assert.Equal(t, "w1", hierarchy.gotRenameID,
		"and the workspace it holds is what gets renamed")
	assert.Equal(t, "feature/x", hierarchy.gotRenameTo)
	assert.Contains(t, rec.Body.String(), "c1",
		"the answer names the CHAT — past law 1 a workspace has no id a client may hold")
}

// The same call the :wsId PATCH's rename half makes, proven by making both and
// comparing — not by asserting a hard-coded expectation twice.
func TestChatRenameBranch_ReachesTheSameUsecaseCallAsTheWorkspaceRoute(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{renamed: domain.Workspace{ID: "w1", Branch: "feature/x"}}
	r, _ := newPairedRouter(&fakeReader{}, hierarchy, heldWorkspace())

	require.Equal(t, http.StatusOK,
		do(r, http.MethodPatch, wsBase, `{"branch":"feature/x"}`).Code)
	viaWorkspace := hierarchy.gotRenameID + "/" + hierarchy.gotRenameTo
	hierarchy.gotRenameID, hierarchy.gotRenameTo = "", ""

	require.Equal(t, http.StatusOK,
		do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`).Code)
	viaChat := hierarchy.gotRenameID + "/" + hierarchy.gotRenameTo

	assert.Equal(t, viaWorkspace, viaChat, "one verb, two keyings, no second implementation")
}

// Every refusal guardRenameBranch makes — a locked branch, a repo-wide name
// collision, an unprovisioned placeholder, an adopted checkout the user owns —
// must arrive through the chat door exactly as it does through the workspace
// one. This is the assertion that the guards were CARRIED OVER rather than
// re-implemented: they all live below the handler, so all this has to prove is
// that the handler still routes through them.
func TestChatRenameBranch_CarriesEveryGuardTheWorkspaceRouteEnforces(
	t *testing.T,
) {
	for name, refusal := range map[string]error{
		"locked branch":             workspace.ErrWorkspaceLocked,
		"branch name taken":         workspace.ErrBranchWorkspaceExists,
		"unprovisioned placeholder": workspace.ErrParentUnprovisioned,
		"adopted checkout":          workspace.ErrRenameUnmanagedWorkspace,
	} {
		t.Run(name, func(t *testing.T) {
			r, _ := newPairedRouter(
				&fakeReader{}, &fakeHierarchy{renameErr: refusal}, heldWorkspace())

			rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`)

			assert.Equal(t, http.StatusConflict, rec.Code)
			assert.Contains(t, rec.Body.String(), refusal.Error())
		})
	}
}

// A blank name never reaches the usecase, the same refusal the :wsId route
// makes — and it is refused for what is wrong with it rather than for a chat
// lookup the request never got to.
func TestChatRenameBranch_BlankBranchIsRefusedBeforeTheUsecase(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newPairedRouter(&fakeReader{}, hierarchy, heldWorkspace())

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"   "}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotRenameID, "a blank name must never reach the usecase")
}

// A malformed body is refused BEFORE the chat is resolved, matching lock's own
// order and the :wsId route's: a body that cannot be parsed is not yet a
// request about any particular chat.
func TestChatRenameBranch_BadJSONIsRefusedBeforeTheChatIsResolved(
	t *testing.T,
) {
	worktrees := heldWorkspace()
	r, _ := newPairedRouter(&fakeReader{}, &fakeHierarchy{}, worktrees)

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, worktrees.gotChat, "nothing was resolved for an unparseable request")
}

// A chat whose worktree cannot be resolved — an unknown id, or a bubble hanging
// off nothing — is the same 404 every other chat-keyed verb answers.
func TestChatRenameBranch_UnresolvableChatIs404(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	r, _ := newPairedRouter(
		&fakeReader{}, hierarchy, &fakeWorktrees{err: worktree.ErrNoWorktreeInAncestry})

	rec := do(r, http.MethodPatch, chatBase+"/branch", `{"branch":"feature/x"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, hierarchy.gotRenameID, "nothing may be renamed for a chat holding nothing")
}
