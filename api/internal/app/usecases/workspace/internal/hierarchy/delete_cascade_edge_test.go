package hierarchy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestDeleteCascade_RootNotFound(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "other"}}, nil
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
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
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
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

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"})
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, deleted, "deepest-first, and each id exactly once")
	assert.Equal(t, 2, countOp(g.ops(), "WorktreeRemove"), "each workspace's worktree is removed exactly once")
}

// A workspace that could not be tombstoned does not stop the others, but it is
// reported: the caller must keep the repo row while one of its workspaces is live.
func TestDeleteRepoWorkspaces_AFailedTombstoneIsReportedAfterTheRest(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "a", WorktreePath: "/wt/a/worktree"},
		{ID: "w2", RepoID: "r1", ProjectID: "p1", Branch: "b", WorktreePath: "/wt/b/worktree"},
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

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"})
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
				{ID: "w1", RepoID: "r1", Branch: "develop", WorktreePath: "/managed"},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "w1"))
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
	all := []domain.Workspace{{ID: "root", RepoID: "r", WorktreePath: "/wt", Branch: "b"}}
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

	require.NoError(t, uc.DeleteCascade(context.Background(), "root"),
		"a terminal-kill failure must not abort the cascade (best-effort)")
	assert.True(t, deleted, "the workspace row is still dropped")
	assert.Equal(t, []string{"chat-root"}, reaper.listed,
		"the reap fans out through the workspace's owning chat, which is where sessions are keyed")
}
