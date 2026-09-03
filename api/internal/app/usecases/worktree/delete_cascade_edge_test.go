package worktree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestDeleteCascade_RootNotFound(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "other"}}, nil
		},
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	err := uc.DeleteCascade(context.Background(), "missing-root")
	require.ErrorIs(t, err, apperr.ErrNotFound)
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
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	err := uc.DeleteCascade(context.Background(), "root")
	require.ErrorIs(t, err, errBoom)
}

// TestDeleteRepoWorkspaces_SkipsNonRootWorkspaces proves the documented
// "walks ROOTS only" contract: a workspace whose parent is ALSO one of the
// repo's own workspaces is never itself an iteration start — it is still
// removed, but exactly once, as part of its root's cascade.
func TestDeleteRepoWorkspaces_SkipsNonRootWorkspaces(t *testing.T) {
	all := []domain.Workspace{
		{ID: "root", RepoID: "r1", ProjectID: "p1", Branch: "b-root", WorktreePath: "/wt/root/worktree"},
		{ID: "child", ParentID: "root", RepoID: "r1", ProjectID: "p1", Branch: "b-child", WorktreePath: "/wt/child/worktree"},
	}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	handled, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, handled, "deepest-first, and each id exactly once")
	assert.Equal(t, 2, countOp(g.ops(), "WorktreeRemove"), "each workspace's worktree is removed exactly once")
}

func TestDeleteRepoWorkspaces_RemoveErrorIsBestEffort(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", WorktreePath: "/wt/a/worktree"},
	}
	ws := &fakeWorkspace{
		ListFn:   func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error { return errBoom },
	}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	handled, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.NoError(t, err, "a per-workspace removal failure must not fail the whole sweep")
	assert.Equal(t, []string{"w1"}, handled)
}

// TestRemoveOne_DefaultBranchReattachFails_IsBestEffort proves a failed
// re-attach of the main folder to the default branch never aborts the cascade
// — the row is still dropped and the branch is still never force-deleted.
func TestRemoveOne_DefaultBranchReattachFails_IsBestEffort(t *testing.T) {
	g := &fakeGit{checkoutErr: errBoom}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "develop", WorktreePath: "/managed"},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "w1"))
	assert.Contains(t, g.ops(), "CheckoutBranch")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the default branch must never be force-deleted even on a failed reattach")
}

// erroringTerminalReaper always fails to kill a session, so
// TestDeleteCascade_TerminalKillError_IsBestEffort can prove the cascade
// still completes.
type erroringTerminalReaper struct {
	byWorkspace map[string][]string
}

func (r *erroringTerminalReaper) ListSessionsForWorkspace(wsID string) []string {
	return r.byWorkspace[wsID]
}

func (r *erroringTerminalReaper) Kill(_ context.Context, _ string) error {
	return errBoom
}

func TestDeleteCascade_TerminalKillError_IsBestEffort(t *testing.T) {
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
	deleted := false
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, _ string) error {
			deleted = true
			return nil
		},
	}
	reaper := &erroringTerminalReaper{byWorkspace: map[string][]string{"root": {"sess-1"}}}
	uc := worktree.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome(),
		worktree.WithTerminalReaper(reaper))

	require.NoError(t, uc.DeleteCascade(context.Background(), "root"),
		"a terminal-kill failure must not abort the cascade (best-effort)")
	assert.True(t, deleted, "the workspace row is still dropped")
}
