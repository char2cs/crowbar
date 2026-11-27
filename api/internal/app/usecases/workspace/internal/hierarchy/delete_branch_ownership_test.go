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

func TestRegression_DeleteRepoWorkspaces_KeepsTheDefaultBranch(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "w-dev", RepoID: "r1", Branch: "develop", WorktreePath: "/tmp/crowbar-test/projects/p/app/dev/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Head: "tip"}}, revParseSha: "tip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteRepoWorkspaces(context.Background(),
		domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "develop"}, domain.KeepWorkAtRisk))

	assert.Equal(t, []string{"w-dev"}, *deleted)
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the default branch is never deleted")
	assert.Contains(t, g.ops(), "CheckoutBranch", "the main folder is re-attached to it instead")
}

func TestRegression_DeleteRepoWorkspaces_NeverForcesALockedWorktree(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{
			ID: "w-main", RepoID: "r1", Branch: "main", Status: domain.WorkspaceStatusLocked,
			WorktreePath: "/tmp/crowbar-test/projects/p/app/main/worktree",
			Provisioning: domain.WorkspaceProvisioned,
		},
		{
			ID: "w-rel", RepoID: "r1", Branch: "release", Status: domain.WorkspaceStatusLocked,
			WorktreePath: "/tmp/crowbar-test/projects/p/app/rel/worktree", CreatedBranch: true,
			Provisioning: domain.WorkspaceProvisioned,
		},
	})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteRepoWorkspaces(context.Background(),
		domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}, domain.KeepWorkAtRisk))

	assert.ElementsMatch(t, []string{"w-main", "w-rel"}, *deleted, "the repo's locked rows go with it")
	assert.Equal(t, "false", removeCall(g, "/tmp/crowbar-test/projects/p/app/main/worktree")[2], "a locked worktree is never --forced")
	assert.Equal(t, "false", removeCall(g, "/tmp/crowbar-test/projects/p/app/rel/worktree")[2], "a locked worktree is never --forced")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "a locked branch is never deleted")
}

func TestDeleteCascade_DeletesOnlyTheBranchCrowbarCreated(t *testing.T) {
	ws, _ := repoDeleteFixture([]domain.Workspace{
		{ID: "root", RepoID: "r1", Branch: "mine", WorktreePath: "/tmp/crowbar-test/projects/p/app/root/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "adopted", ParentID: "root", RepoID: "r1", Branch: "theirs", WorktreePath: "/tmp/crowbar-test/projects/p/app/adopted/worktree", Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"},
		newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk))

	var branches []string
	for _, c := range g.calls {
		if c.op == "ForceDeleteBranch" {
			branches = append(branches, c.args[1])
		}
	}
	assert.Equal(t, []string{"mine"}, branches)
	assert.Equal(t, "true", removeCall(g, "/tmp/crowbar-test/projects/p/app/adopted/worktree")[2],
		"an ordinary unlocked workspace is still removed with --force")
}

func TestRegression_DeleteCascade_NeverForcesACheckoutHoldingASiblingsChats(t *testing.T) {
	ws, _ := repoDeleteFixture([]domain.Workspace{
		{ID: "chats", RepoID: "r1", Branch: "chats", WorktreePath: "/tmp/crowbar-test/projects/p/app/chats", Provisioning: domain.WorkspaceProvisioned},
		{ID: "dev", RepoID: "r1", Branch: "dev", WorktreePath: "/tmp/crowbar-test/projects/p/app/dev", Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"},
		newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "chats", domain.KeepWorkAtRisk))

	assert.Equal(t, "false", removeCall(g, "/tmp/crowbar-test/projects/p/app/chats")[2])
}

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

func TestRemoveOne_LeavesAMainFolderCrowbarDidNotDetach(t *testing.T) {
	for name, main := range map[string]enginegit.WorktreeEntry{
		"on another branch": {Path: "/repo", Branch: "master", Head: "tip"},
		"moved since":       {Path: "/repo", Head: "users-own-commit"},
	} {
		t.Run(name, func(t *testing.T) {
			ws, _ := repoDeleteFixture([]domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "main", WorktreePath: "/tmp/crowbar-test/projects/p/app/main/worktree", Provisioning: domain.WorkspaceProvisioned},
			})
			g := &fakeGit{worktrees: []enginegit.WorktreeEntry{main}, revParseSha: "tip"}
			uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"},
				newNow(), fakeHome())

			require.NoError(t, uc.DeleteCascade(context.Background(), "w1", domain.KeepWorkAtRisk))
			assert.NotContains(t, g.ops(), "CheckoutBranch")
			assert.NotContains(t, g.ops(), "ForceDeleteBranch")
		})
	}
}
