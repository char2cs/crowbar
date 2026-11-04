package hierarchy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// A delete never destroys work that exists nowhere else unless the request
// carries consent: without it the whole delete stops, untouched, and names it.

const managed = "/tmp/crowbar-test/projects/p/app/"

func TestDeleteCascade_WithoutConsentRefusesOverUnmergedCommitsAndTouchesNothing(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "root", RepoID: "r1", Branch: "feature/a", WorktreePath: managed + "a/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "child", ParentID: "root", RepoID: "r1", Branch: "feature/b", WorktreePath: managed + "b/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{unmerged: map[string]int{"feature/b": 2}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"}, newNow(), fakeHome())

	err := uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Equal(t, map[string]any{"workAtRisk": []domain.WorkAtRisk{{WorkspaceID: "child", Branch: "feature/b", UnmergedCommits: 2}}},
		problem.DataOf(err))
	assert.Empty(t, *deleted, "no row goes")
	assert.Zero(t, countOp(g.ops(), "WorktreeRemove"), "no worktree goes")
	assert.Zero(t, countOp(g.ops(), "ForceDeleteBranch"), "no branch goes")
}

func TestDeleteCascade_WithConsentDiscardsWorkAtRisk(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "root", RepoID: "r1", Branch: "feature/a", WorktreePath: managed + "a/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{unmerged: map[string]int{"feature/a": 1}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.DiscardWorkAtRisk))

	assert.Equal(t, []string{"root"}, *deleted)
	assert.Equal(t, 1, countOp(g.ops(), "ForceDeleteBranch"))
}

// Uncommitted work counts only in a live worktree that would be FORCE-removed.
func TestDeleteRepoWorkspaces_WithoutConsentRefusesOverUncommittedWork(t *testing.T) {
	home := t.TempDir()
	dirty := filepath.Join(home, "projects", "p", "app", "dirty", "worktree")
	require.NoError(t, os.MkdirAll(dirty, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dirty, ".git"), []byte("gitdir: x\n"), 0o600))
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "w1", RepoID: "r1", Branch: "feature/dirty", WorktreePath: dirty, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{uncommitted: map[string]int{dirty: 3}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(),
		func() (string, error) { return home, nil })
	repo := domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}

	risks, err := uc.RepoWorkAtRisk(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, []domain.WorkAtRisk{{WorkspaceID: "w1", Branch: "feature/dirty", UncommittedFiles: 3}}, risks)

	require.ErrorIs(t, uc.DeleteRepoWorkspaces(context.Background(), repo, domain.KeepWorkAtRisk), domain.ErrWorkAtRisk)
	assert.Empty(t, *deleted)
	assert.Zero(t, countOp(g.ops(), "WorktreeRemove"))
}

// Work git cannot assess is not assumed safe.
func TestDeleteCascade_AnUnassessableWorkspaceStopsTheDeleteWithoutConsent(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "root", RepoID: "r1", Branch: "feature/a", WorktreePath: managed + "a/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{riskErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"}, newNow(), fakeHome())

	require.ErrorIs(t, uc.DeleteCascade(context.Background(), "root", domain.KeepWorkAtRisk), errBoom)
	assert.Empty(t, *deleted)

	require.NoError(t, uc.DeleteCascade(context.Background(), "root", domain.DiscardWorkAtRisk))
	assert.Equal(t, []string{"root"}, *deleted)
}

// The repo's main checkout, and any checkout outside <home>/projects, is the
// user's: its row goes with the repo, but git is never asked to remove it —
// not even with consent, and not trusting git to refuse.
func TestDeleteRepoWorkspaces_NeverRemovesACheckoutCrowbarDidNotCreate(t *testing.T) {
	ws, deleted := repoDeleteFixture([]domain.Workspace{
		{ID: "home", RepoID: "r1", Branch: "main", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "linked", RepoID: "r1", Branch: "users-own", WorktreePath: "/elsewhere/linked", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "adopted", RepoID: "r1", Branch: "adopted", WorktreePath: managed + "adopted/worktree", Provisioning: domain.WorkspaceShared},
		{ID: "made", RepoID: "r1", Branch: "feature/made", WorktreePath: managed + "made/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	require.NoError(t, uc.DeleteRepoWorkspaces(context.Background(),
		domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}, domain.DiscardWorkAtRisk))

	assert.ElementsMatch(t, []string{"home", "linked", "adopted", "made"}, *deleted)
	var removed []string
	for _, c := range g.calls {
		if c.op == "WorktreeRemove" {
			removed = append(removed, c.args[1])
		}
	}
	assert.Equal(t, []string{managed + "made/worktree"}, removed)
}

func TestWorkAtRisk_ListsEachDoomedWorkspaceOnce(t *testing.T) {
	ws, _ := repoDeleteFixture([]domain.Workspace{
		{ID: "root", RepoID: "r1", Branch: "feature/a", WorktreePath: managed + "a/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
		{ID: "child", ParentID: "root", RepoID: "r1", Branch: "feature/b", WorktreePath: managed + "b/worktree", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
	})
	g := &fakeGit{unmerged: map[string]int{"feature/a": 1, "feature/b": 4}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", defaultBranch: "main"}, newNow(), fakeHome())

	risks, err := uc.WorkAtRisk(context.Background(), []string{"root", "child", "unknown"})

	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.WorkAtRisk{
		{WorkspaceID: "root", Branch: "feature/a", UnmergedCommits: 1},
		{WorkspaceID: "child", Branch: "feature/b", UnmergedCommits: 4},
	}, risks)
}

// TestRemoveOne_DefaultBranchWorkspace_ReattachesMainAndKeepsBranch proves that
// removing a managed workspace on the repo's default branch re-attaches the main
// folder to that branch and NEVER force-deletes it (the shared integration
// branch must survive).
func TestRemoveOne_DefaultBranchWorkspace_ReattachesMainAndKeepsBranch(t *testing.T) {
	// The main folder is exactly as a create's detach left it: detached at the tip.
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Head: "tip"}}, revParseSha: "tip"}
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

	assert.Contains(t, g.ops(), "CheckoutBranch", "the main folder must be re-attached to the default branch")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the default branch must never be force-deleted")
}

// TestRemoveOne_FeatureBranchWorkspace_ForceDeletesBranch proves removing a
// managed workspace on a non-default branch still force-deletes that branch (the
// default-branch protection must not leak into ordinary teardown).
func TestRemoveOne_FeatureBranchWorkspace_ForceDeletesBranch(t *testing.T) {
	g := &fakeGit{}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "w1", RepoID: "r1", Branch: "feature/x", WorktreePath: "/managed", CreatedBranch: true, Provisioning: domain.WorkspaceProvisioned},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "w1", domain.KeepWorkAtRisk))

	assert.Contains(t, g.ops(), "ForceDeleteBranch", "a feature branch is force-deleted on teardown")
	assert.NotContains(t, g.ops(), "CheckoutBranch", "no re-attach for a non-default branch")
}

// TestRemoveOne_PlaceholderSkipsGitTeardown proves a placeholder (empty
// WorktreePath) whose branch is a protected NON-default branch is torn down as a
// pure read-model drop: no WorktreeRemove, no ForceDeleteBranch, no CheckoutBranch
// — the real branch is never git-touched (spec §5 defense-in-depth). The
// placeholder is created NON-locked here only to exercise removeOne directly
// (DeleteCascade's locked guard is proven separately by TestDeleteCascade).
func TestRemoveOne_PlaceholderSkipsGitTeardown(t *testing.T) {
	g := &fakeGit{}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "ph", RepoID: "r1", Branch: "master", Provisioning: domain.WorkspacePlaceholder},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "ph", domain.KeepWorkAtRisk))

	assert.NotContains(t, g.ops(), "WorktreeRemove")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the real protected branch must never be force-deleted")
	assert.NotContains(t, g.ops(), "CheckoutBranch")
}
