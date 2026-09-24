package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// A teardown removes only what Crowbar made (spec §3 P0-1, invariant D5). These
// pin the ways a repo or workspace delete used to destroy the user's own git
// state.

func repoDeleteFixture(all []domain.Workspace) (*fakeWorkspace, *[]string) {
	deleted := []string{}
	return &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}, &deleted
}

func removeCall(g *fakeGit, path string) []string {
	for _, c := range g.calls {
		if c.op == "WorktreeRemove" && c.args[1] == path {
			return c.args
		}
	}
	return nil
}

// The repo row is gone by the time its cascade runs, so the default branch has
// to come from the caller. Without it every managed workspace looked like a
// feature branch and the default branch was force-deleted.
func TestRegression_DeleteRepoWorkspaces_KeepsTheDefaultBranch(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "w-dev", RepoID: "r1", Branch: "develop", WorktreePath: "/wt/dev/worktree", CreatedBranch: true},
	})
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Head: "tip"}}, revParseSha: "tip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteRepoWorkspaces(context.Background(),
		domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "develop"}))

	assert.Equal(t, []string{"w-dev"}, *deleted)
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the default branch is never deleted")
	assert.Contains(t, g.ops(), "CheckoutBranch", "the main folder is re-attached to it instead")
}

// A locked (protected) worktree may hold uncommitted work that is not
// Crowbar's to throw away: it is removed without --force, so git refuses a
// dirty one, and its branch — which Crowbar never created — survives.
func TestRegression_DeleteRepoWorkspaces_NeverForcesALockedWorktree(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "w-main", RepoID: "r1", Branch: "main", Status: domain.WorkspaceStatusLocked,
			WorktreePath: "/wt/main/worktree"},
		{ID: "w-rel", RepoID: "r1", Branch: "release", Status: domain.WorkspaceStatusLocked,
			WorktreePath: "/wt/rel/worktree", CreatedBranch: true},
	})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteRepoWorkspaces(context.Background(),
		domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}))

	assert.ElementsMatch(t, []string{"w-main", "w-rel"}, *deleted, "the repo's locked rows go with it")
	assert.Equal(t, "false", removeCall(g, "/wt/main/worktree")[2], "a locked worktree is never --forced")
	assert.Equal(t, "false", removeCall(g, "/wt/rel/worktree")[2], "a locked worktree is never --forced")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "a locked branch is never deleted")
}

// Only a branch Crowbar created goes with its workspace. A row that adopted a
// branch which already existed keeps it — and so does every row written before
// CreatedBranch was recorded, which errs toward keeping.
func TestDeleteCascade_DeletesOnlyTheBranchCrowbarCreated(t *testing.T) {
	ws, _ := repoDeleteFixture([]domain.Workspace{
		{ID: "root", RepoID: "r1", Branch: "mine", WorktreePath: "/wt/root/worktree", CreatedBranch: true},
		{ID: "adopted", ParentID: "root", RepoID: "r1", Branch: "theirs", WorktreePath: "/wt/adopted/worktree"},
	})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"},
		newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "root"))

	var branches []string
	for _, c := range g.calls {
		if c.op == "ForceDeleteBranch" {
			branches = append(branches, c.args[1])
		}
	}
	assert.Equal(t, []string{"mine"}, branches)
	assert.Equal(t, "true", removeCall(g, "/wt/adopted/worktree")[2],
		"an ordinary unlocked workspace is still removed with --force")
}

// A failed create only takes back a branch it made: the -B import of a remote
// branch that already existed locally must not delete the user's local branch
// when the row then fails to land.
func TestCreateChild_FailedImportKeepsAPreexistingLocalBranch(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseSha: "tip"}
	ws := &fakeWorkspace{
		CreateFn: func(context.Context, workspace.CreateInput, time.Time) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "feature/x",
		ParentID: "w-parent", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, g.ops(), "WorktreeRemove", "the half-made worktree is still cleaned up")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "but the user's own branch is not")
}

// Re-attaching the main folder is undoing a detach Crowbar made. A folder on a
// branch of its own, or a detached HEAD the user has since moved, is the
// user's checkout: removing a default-branch workspace must not switch it.
func TestRemoveOne_LeavesAMainFolderCrowbarDidNotDetach(t *testing.T) {
	for name, main := range map[string]enginegit.WorktreeEntry{
		"on another branch": {Path: "/repo", Branch: "master", Head: "tip"},
		"moved since":       {Path: "/repo", Head: "users-own-commit"},
	} {
		t.Run(name, func(t *testing.T) {
			ws, _ := repoDeleteFixture([]domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "main", WorktreePath: "/wt/main/worktree"},
			})
			g := &fakeGit{worktrees: []enginegit.WorktreeEntry{main}, revParseSha: "tip"}
			uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"},
				newNow(), fakeHome())

			require.NoError(t, uc.DeleteCascade(context.Background(), "w1"))
			assert.NotContains(t, g.ops(), "CheckoutBranch")
			assert.NotContains(t, g.ops(), "ForceDeleteBranch")
		})
	}
}
