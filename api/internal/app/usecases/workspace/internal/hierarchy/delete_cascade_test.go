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

// TestDeleteCascade_RemoveOneError_Propagates proves that (unlike a git-side
// teardown failure, which is best-effort) a failure to drop the READ-MODEL row
// itself is NOT swallowed — DeleteCascade must report it rather than claim
// success for a workspace that is still listed.
func TestDeleteCascade_RemoveOneError_Propagates(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r"}} // no WorktreePath: skips git entirely
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return errBoom },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	err := uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk)
	require.ErrorIs(t, err, errBoom)
}

// TestDeleteRepoWorkspaces_SkipsNonRootWorkspaces proves the documented
// "walks ROOTS only" contract: a workspace whose parent is ALSO one of the
// repo's own workspaces is never itself an iteration start — it is still
// removed, but exactly once, as part of its root's cascade.
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

// A workspace that could not be tombstoned does not stop the others, but it is
// reported: the caller must keep the repo row while one of its workspaces is live.
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

// TestRemoveOne_DefaultBranchReattachFails_IsBestEffort proves a failed
// re-attach of the main folder to the default branch never aborts the cascade
// — the row is still dropped and the branch is still never force-deleted.
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

// erroringTerminalReaper always fails to kill a session, so
// TestDeleteCascade_TerminalKillError_IsBestEffort can prove the cascade
// still completes.
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

// A locked child the cascade keeps is re-hung under its deleted parent's
// nearest surviving ancestor, so no lineage names a workspace that is gone.
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

// DeletionPlan answers what DeleteCascade would take, or its refusal, and takes nothing.
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

// TestDeleteCascade_KillsTerminalSessions proves pass-7: every cascade-deleted
// workspace's live PTY sessions are terminated, so deleting a workspace (or a
// whole subtree) never leaks shell processes, fds, or per-session ring buffers.
//
// Sessions are keyed by their OWNING CHAT, so the reap fans out through the chat
// observer: each workspace's chats are listed, and each chat's own sessions are
// killed. The observer is therefore load-bearing here — without it the reap has
// no chat to ask and kills nothing.
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

// TestDeleteCascade_SkipsLockedStatus proves the cascade planner derives the
// skip-locked rule from Status==locked (not the legacy Locked bool): the
// locked-status node 'b' is preserved while its unlocked descendant 'c' is
// still deleted, and 'b' is re-hung atop its lineage.
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

// TestDeleteCascade_RejectsLockedRootStatus proves the root-level guard now
// derives from Status==locked rather than the legacy Locked bool.
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

// A repo's main checkout and a project's home are served with an owning chat,
// so deleting that owner reaches DeleteCascade with either as root.
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

// TestDeleteCascade_RefusesAWorkingChat closes invariant 9's own "no bypass"
// hole: DeleteCascade is the verb behind the live DELETE .../workspaces/:wsId
// route (the sidebar's removal tray) and behind merge --deleteSource, and it
// checked only WorkspaceStatusLocked — so a subtree the chat tree's own delete
// would refuse could be erased whole through the workspace door instead. It
// now takes the SAME guardNotWorking Task 16 built for guardReparent.
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

// TestDeleteCascade_AllowsAnIdleChat proves the guard is not a blanket
// refusal — a subtree whose chats are all idle deletes exactly as before.
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

// H17: removeOne is best-effort — a repo-path / worktree / branch teardown
// failure must drop the read-model row anyway (the user asked to delete it),
// never abort the cascade and never leave a ghost row pointing at a gone
// worktree. Orphaned worktrees on disk are reaped by `git worktree prune`.

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
	// FindByKey returns (nil, nil) for a missing repo row (must not panic); the
	// repo is gone so the worktree is unreachable, so the row is dropped best-effort.
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
