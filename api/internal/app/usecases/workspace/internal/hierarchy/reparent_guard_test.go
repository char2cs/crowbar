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
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, child, nil)
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "c")
	require.ErrorIs(t, err, hierarchy.ErrSelfParent)
	assert.Empty(t, g.calls)
}

func TestReparent_SelfLoopedChildIsStillALeaf(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, parentID, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id, ParentID: parentID}, nil
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
}

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
