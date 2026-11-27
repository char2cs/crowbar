package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestDeleteCascade_RootNotFound(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "other"}}, nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	err := uc.DeleteCascade(context.Background(), "missing-root", domain.KeepWorkAtRisk)
	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestDeleteCascade_RemoveOneError_Propagates(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r"}}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return errBoom },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	err := uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk)
	require.ErrorIs(t, err, errBoom)
}

func TestDeleteRepoWorkspaces_SkipsNonRootWorkspaces(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r1", ProjectID: "p1", Branch: "b-root", WorktreePath: "/tmp/crowbar-test/projects/p/app/root/worktree", Provisioning: domain.WorkspaceProvisioned},
		{ID: "child", ParentID: "root", RepoID: "r1", ProjectID: "p1", Branch: "b-child", WorktreePath: "/tmp/crowbar-test/projects/p/app/child/worktree", Provisioning: domain.WorkspaceProvisioned},
	}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, deleted, "deepest-first, and each id exactly once")
	assert.Equal(t, 2, countOp(g.ops(), "WorktreeRemove"), "each workspace's worktree is removed exactly once")
}

func TestDeleteRepoWorkspaces_AFailedTombstoneIsReportedAfterTheRest(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "a", WorktreePath: "/tmp/crowbar-test/projects/p/app/a/worktree", Provisioning: domain.WorkspaceProvisioned},
		{ID: "w2", RepoID: "r1", ProjectID: "p1", Branch: "b", WorktreePath: "/tmp/crowbar-test/projects/p/app/b/worktree", Provisioning: domain.WorkspaceProvisioned},
	}
	var tried []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			tried = append(tried, id)
			if id == "w1" {
				return errBoom
			}
			return nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.ErrorIs(t, err, errBoom)
	assert.ElementsMatch(t, []string{"w1", "w2"}, tried)
}

func TestRemoveOne_DefaultBranchReattachFails_IsBestEffort(t *testing.T) {
	g := &fakeGit{checkoutErr: errBoom, worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Head: "tip"}}, revParseSha: "tip"}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "develop", WorktreePath: "/managed", Provisioning: domain.WorkspaceProvisioned},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "w1", domain.KeepWorkAtRisk))
	assert.Contains(t, g.ops(), "CheckoutBranch")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the default branch must never be force-deleted even on a failed reattach")
}

type erroringTerminalReaper struct {
	byChat map[string][]string
	listed []string
}

func (r *erroringTerminalReaper) ListSessionsForChat(chatID string) []string {
	r.listed = append(r.listed, chatID)
	return r.byChat[chatID]
}

func (r *erroringTerminalReaper) Kill(_ context.Context, _ string) error {
	return errBoom
}

func TestDeleteCascade_TerminalKillError_IsBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b", Provisioning: domain.WorkspaceProvisioned}}
	deleted := false
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error {
			deleted = true
			return nil
		},
	}
	reaper := &erroringTerminalReaper{byChat: map[string][]string{"chat-root": {"sess-1"}}}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(),
		hierarchy.WithTerminalReaper(reaper))
	uc.SetChatObserver(&fakeChatObserver{chats: []domain.Chat{{ID: "chat-root", WorkspaceID: "root"}}})

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk),
		"a terminal-kill failure must not abort the cascade (best-effort)")
	assert.True(t, deleted, "the workspace row is still dropped")
	assert.Equal(t, []string{"chat-root"}, reaper.listed,
		"the reap fans out through the workspace's owning chat, which is where sessions are keyed")
}

func TestDeleteCascade_RehangsAKeptLockedChildUnderTheNearestSurvivor(t *testing.T) {
	all := []domain.Workspace{
		{ID: "top", RepoID: "r"},
		{ID: "root", ParentID: "top", RepoID: "r"},
		{ID: "mid", ParentID: "root", RepoID: "r"},
		{ID: "kept", ParentID: "mid", RepoID: "r", Status: domain.WorkspaceStatusLocked, ForkPointSha: "fp"},
	}
	rehung := map[string]string{}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return nil },
		ReparentFn: func(_ context.Context, id, parentID, forkPoint string, _ time.Time) (domain.Workspace, error) {
			rehung[id] = parentID + "@" + forkPoint
			return domain.Workspace{}, nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))

	assert.Equal(t, map[string]string{"kept": "top@fp"}, rehung)
}

func TestDeletionPlan_ListsWhatGoesOrRefusesWithoutTouchingAnything(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r"},
		{ID: "child", ParentID: "root", RepoID: "r"},
		{ID: "locked", ParentID: "root", RepoID: "r", Status: domain.WorkspaceStatusLocked},
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			t.Fatalf("DeletionPlan deleted %s", id)
			return nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	ids, err := uc.DeletionPlan(context.Background(), "root")
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, ids)
	_, err = uc.DeletionPlan(context.Background(), "locked")
	require.ErrorIs(t, err, hierarchy.ErrWorkspaceLocked)
}

type fakeTerminalReaper struct {
	byChat map[string][]string
	killed []string
}

func (f *fakeTerminalReaper) ListSessionsForChat(chatID string) []string {
	return f.byChat[chatID]
}

func (f *fakeTerminalReaper) Kill(_ context.Context, sid string) error {
	f.killed = append(f.killed, sid)
	return nil
}

func TestDeleteCascade_KillsTerminalSessions(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/tmp/crowbar-test/projects/p/app/root", Provisioning: domain.WorkspaceProvisioned},
		{ID: "child", ParentID: "root", RepoID: "r", Branch: "b-child", WorktreePath: "/tmp/crowbar-test/projects/p/app/child", Provisioning: domain.WorkspaceProvisioned},
	}
	g := &fakeGit{}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	reaper := &fakeTerminalReaper{byChat: map[string][]string{
		"chat-root":  {"root-sess"},
		"chat-child": {"child-sess-1", "child-sess-2"},
	}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(),
		hierarchy.WithTerminalReaper(reaper))
	uc.SetChatObserver(&fakeChatObserver{chats: []domain.Chat{
		{ID: "chat-root", WorkspaceID: "root"},
		{ID: "chat-child", WorkspaceID: "child"},
	}})

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.ElementsMatch(t, []string{"root-sess", "child-sess-1", "child-sess-2"}, reaper.killed,
		"every cascade-deleted workspace's terminal sessions must be killed")
}

func TestDeleteCascade_SkipsLockedStatus(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/tmp/crowbar-test/projects/p/app/root", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "a", ParentID: "root", RepoID: "r", Branch: "b-a", WorktreePath: "/tmp/crowbar-test/projects/p/app/a", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "b", ParentID: "a", Status: domain.WorkspaceStatusLocked, RepoID: "r", Branch: "b-b", WorktreePath: "/tmp/crowbar-test/projects/p/app/b", Provisioning: domain.WorkspaceProvisioned},
		{ID: "c", ParentID: "b", RepoID: "r", Branch: "b-c", WorktreePath: "/tmp/crowbar-test/projects/p/app/c", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	}
	g := &fakeGit{}
	var deleted []string
	rehung := map[string]string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
		ReparentFn: func(_ context.Context, id, parentID, _ string, _ time.Time) (domain.Workspace, error) {
			rehung[id] = parentID
			return domain.Workspace{}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"c", "a", "root"}, deleted)
	assert.Equal(t, map[string]string{"b": ""}, rehung, "the kept locked b tops its lineage")
	assert.Equal(t, []string{
		"WorktreeRemove", "ForceDeleteBranch",
		"WorktreeRemove", "ForceDeleteBranch",
		"WorktreeRemove", "ForceDeleteBranch",
	}, g.ops())
	assert.Equal(t, []string{"/repo", "/tmp/crowbar-test/projects/p/app/c", "true"}, g.calls[0].args)
	assert.Equal(t, []string{"/repo", "b-c"}, g.calls[1].args)
}

func TestDeleteCascade_RejectsLockedRootStatus(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Status: domain.WorkspaceStatusLocked, Branch: "b", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned},
	}
	g := &fakeGit{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk), hierarchy.ErrWorkspaceLocked)
	assert.Empty(t, g.calls)
}

func TestRegression_DeleteCascade_RefusesTheDefaultCheckoutAndTheHome(t *testing.T) {
	for want, root := range map[error]domain.Workspace{
		hierarchy.ErrWorkspaceIsDefault: {ID: "root", RepoID: "r", IsDefault: true, Status: domain.WorkspaceStatusNew, Branch: "main", WorktreePath: "/repo", Provisioning: domain.WorkspaceShared},
		hierarchy.ErrWorkspaceIsHome:    {ID: "root", Kind: domain.WorkspaceKindHome, Status: domain.WorkspaceStatusNew, WorktreePath: "/home"},
	} {
		g := &fakeGit{}
		ws := &fakeWorkspace{
			ListFn: func(_ context.Context) ([]domain.Workspace, error) { return []domain.Workspace{root}, nil },
		}
		uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
		require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk), want)
		assert.Empty(t, g.calls)
	}
}

func TestDeleteCascade_RefusesAWorkingChat(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/tmp/crowbar-test/projects/p/app/root", Provisioning: domain.WorkspaceProvisioned},
		{ID: "child", ParentID: "root", RepoID: "r", Branch: "b-child", WorktreePath: "/tmp/crowbar-test/projects/p/app/child", Provisioning: domain.WorkspaceProvisioned},
	}
	g := &fakeGit{}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	uc.SetChatObserver(&fakeChatObserver{
		chats:   []domain.Chat{{ID: "chat-1", WorkspaceID: "child"}},
		working: map[string]bool{"chat-1": true},
	})

	err := uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, hierarchy.ErrWorkspaceWorking)
	assert.Empty(t, g.calls, "a refused cascade must reach no git verb at all")
}

func TestDeleteCascade_AllowsAnIdleChat(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r", Branch: "b-root", WorktreePath: "/tmp/crowbar-test/projects/p/app/root", Provisioning: domain.WorkspaceProvisioned},
	}
	g := &fakeGit{}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	uc.SetChatObserver(&fakeChatObserver{
		chats:   []domain.Chat{{ID: "chat-1", WorkspaceID: "root"}},
		working: map[string]bool{"chat-1": false},
	})

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_ListError(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk), errBoom)
}

func TestDeleteCascade_RepoPathError_DropsRowBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r"}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome())
	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_MissingRepoRow_DropsRowNoPanic(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b", Provisioning: domain.WorkspaceProvisioned}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())
	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_WorktreeRemoveError_DropsRowBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b", Provisioning: domain.WorkspaceProvisioned}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	g := &fakeGit{removeErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"root"}, deleted)
}

func TestDeleteCascade_BranchDeleteError_DropsRowBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b", Provisioning: domain.WorkspaceProvisioned}}
	var deleted []string
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	}
	g := &fakeGit{deleteErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"root"}, deleted)
}
