package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestReparent_AllowsLockedNewParent(t *testing.T) {
	// A locked (protected) branch is a valid re-parent target: it already adopts
	// children via create, so reparent must be consistent — the old "07 §4
	// new-parent-locked" block was incoherent and has been removed.
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
}

func TestReparent_RejectsNonLeafChild(t *testing.T) {
	child := domain.Workspace{ID: "c"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	grandchild := domain.Workspace{ID: "gc", ParentID: "c"}
	ws := reparentWS(child, newParent, []domain.Workspace{grandchild})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, hierarchy.ErrChildHasChildren)
	assert.Empty(t, g.calls)
}

// A worktree-owning child names a specific remote, branch and worktree path —
// moving it under a parent in a DIFFERENT repo is not a reparent at all, so
// guardReparent refuses it before any git work (model spec invariant 7).
func TestReparent_RefusesCrossRepoWhenChildOwnsAWorktree(t *testing.T) {
	child := domain.Workspace{ID: "c", RepoID: "repo-a", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", RepoID: "repo-b", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, hierarchy.ErrCrossRepoWorktreeMove)
	assert.Empty(t, g.calls)
}

// A row with no worktree of its own carries no remote, branch or worktree path
// to lose across a repo boundary, so a cross-repo move is still a plain
// reparent for it: RebasesOntoNewTipAndUpdatesAggregate below already proves
// the happy path within one repo, and the guard itself imposes no same-repo
// requirement when child.WorktreePath is empty (see guardReparent).
func TestReparent_AllowsCrossRepoWhenChildOwnsNoWorktree(t *testing.T) {
	child := domain.Workspace{ID: "c", RepoID: "repo-a", Branch: "feat"}
	newParent := domain.Workspace{ID: "np", RepoID: "repo-b", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
}

func TestReparent_RejectsSelfParent(t *testing.T) {
	// A workspace must never become its own parent: the self-loop detaches the
	// node in the tree and (via childHasChildren) makes it permanently
	// unreparentable. The guard rejects it before any git work.
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, child, nil)
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "c")
	require.ErrorIs(t, err, hierarchy.ErrSelfParent)
	assert.Empty(t, g.calls)
}

func TestReparent_SelfLoopedChildIsStillALeaf(t *testing.T) {
	// A workspace already corrupted into a self-loop (ParentID == ID) must not
	// count as its own child, so the leaf check passes and it can be reparented
	// out of the bad state onto a real parent.
	child := domain.Workspace{ID: "c", ParentID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, parentID, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id, ParentID: parentID}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err) // not blocked by a phantom self-child
}

// TestReparent_RefusesAWorkingChat: a reparent rewrites the workspace's git state,
// so it must refuse before any git work while a chat there is mid-turn — the
// same subtree rule the chat tree's guardNotWorking enforces (invariant 5).
func TestReparent_RefusesAWorkingChat(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	uc.SetChatObserver(&fakeChatObserver{
		chats:   []domain.Chat{{ID: "chat-1", WorkspaceID: "c"}},
		working: map[string]bool{"chat-1": true},
	})

	_, err := uc.Reparent(context.Background(), "c", "np")

	require.ErrorIs(t, err, hierarchy.ErrWorkspaceWorking)
	assert.Empty(t, g.calls)
}

// TestReparent_AllowsAnIdleChat proves the guard is not a blanket refusal: a
// workspace whose chats are all idle reparents exactly as before this task.
func TestReparent_AllowsAnIdleChat(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	uc.SetChatObserver(&fakeChatObserver{
		chats:   []domain.Chat{{ID: "chat-1", WorkspaceID: "c"}},
		working: map[string]bool{"chat-1": false},
	})

	_, err := uc.Reparent(context.Background(), "c", "np")

	require.NoError(t, err)
}

// TestReparent_NoChatObserverWired_GuardIsANoOp proves every existing caller
// that never wires a ChatWorkObserver (every test above this one, and every
// caller before this task) is unaffected: a nil observer never refuses.
func TestReparent_NoChatObserverWired_GuardIsANoOp(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")

	require.NoError(t, err)
}

func TestReparent_GuardListError(t *testing.T) {
	child := domain.Workspace{ID: "c"}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return newParent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

// TestReparent_RejectsUnprovisionedNewParent: reparenting onto a placeholder
// parent is rejected before RevParse.
func TestReparent_RejectsUnprovisionedNewParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "old", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspacePlaceholder}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return newParent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{child, newParent}, nil
		},
	}
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, hierarchy.ErrParentUnprovisioned)
	assert.Empty(t, g.calls)
}
