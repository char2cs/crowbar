package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The project's whole tree goes: icon, repo entity dirs, worktree roots.
func TestProjectDelete_RemovesTheProjectTree(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	projectDir := filepath.Join(f.home, "projects", "p1")
	root := filepath.Join(projectDir, "github.com", "acme", "repo", "feature")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "c1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "r1", "storages"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "icon"), []byte("png"), 0o644))

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.NoDirExists(t, projectDir, "projects/p1 must be gone entirely")
}

// A repo moved to project p2 keeps its worktrees where they were created, under
// projects/p1. Deleting p1 used to rm -rf them — p2's worktrees and chats — with
// it (spec §3 P0-3, invariant D6). They, and only they, survive; so does a
// row whose own ProjectID still says p1 but whose repo is p2's.
func TestRegression_ProjectDelete_NeverRemovesAnotherProjectsWorktree(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	projectDir := filepath.Join(f.home, "projects", "p1")
	moved := filepath.Join(projectDir, "github.com", "acme", "other", "b", "worktree")
	stale := filepath.Join(projectDir, "github.com", "acme", "other", "c", "worktree")
	mine := filepath.Join(projectDir, "github.com", "acme", "repo", "feature", "worktree")
	for _, dir := range []string{moved, stale, mine} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("w"), 0o644))
	}
	movedChats := filepath.Join(filepath.Dir(moved), "chats", "c1")
	require.NoError(t, os.MkdirAll(movedChats, 0o755))
	// A pre-leaf row of the moved repo: its checkout and the shared chats tree.
	preLeaf := filepath.Join(projectDir, "github.com", "acme", "other", "d")
	preLeafChats := filepath.Join(projectDir, "github.com", "acme", "other", "chats", "c2")
	require.NoError(t, os.MkdirAll(preLeaf, 0o755))
	require.NoError(t, os.MkdirAll(preLeafChats, 0o755))
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-preleaf", ProjectID: "p2", RepoID: "r-other", WorktreePath: preLeaf, Provisioning: domain.WorkspaceProvisioned},
		{ID: "w-moved", ProjectID: "p2", RepoID: "r-other", WorktreePath: moved, Provisioning: domain.WorkspaceProvisioned},
		{ID: "w-stale", ProjectID: "p1", RepoID: "r-other", WorktreePath: stale, Provisioning: domain.WorkspaceProvisioned},
		{ID: "w-mine", ProjectID: "p1", RepoID: "r1", WorktreePath: mine, Provisioning: domain.WorkspaceProvisioned},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.FileExists(t, filepath.Join(moved, "work.txt"), "another project's worktree survives")
	assert.DirExists(t, movedChats, "with the chats beside it")
	assert.DirExists(t, preLeaf, "a pre-leaf checkout of another project survives")
	assert.DirExists(t, preLeafChats, "with the chats tree it resolves")
	assert.FileExists(t, filepath.Join(stale, "work.txt"), "a row of another project's repo survives")
	assert.NoDirExists(t, mine, "the project's own worktree goes")
	assert.NotContains(t, f.workspaces.deleted, "w-stale", "and its row is not the project's to tombstone")
}

// A worktree git refused to remove — a protected one with uncommitted work —
// is still a registered checkout. It is not the project delete's to rm -rf:
// that would destroy the work and strand the registration in the user's repo.
func TestRegression_ProjectDelete_KeepsACheckoutGitStillRegisters(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	worktree := filepath.Join(f.home, "projects", "p1", "github.com", "acme", "repo", "main", "worktree")
	require.NoError(t, os.MkdirAll(worktree, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "unsaved.txt"), []byte("w"), 0o644))

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.FileExists(t, filepath.Join(worktree, "unsaved.txt"))
}

// The user's real repository lives outside the crowbar home and is never
// touched, whatever the rows say.
func TestProjectDelete_NeverTouchesTheRealRepoPath(t *testing.T) {
	f := newDeleteFixture(t)
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "README.md"), []byte("hi"), 0o644))
	f.projects.projects["p1"] = domain.Project{ID: "p1", Path: real}
	f.repos.repos = []domain.Repository{{ID: "r1", ProjectID: "p1", Path: real}}
	f.workspaces.workspaces = []domain.Workspace{{ID: "w-home", ProjectID: "p1", WorktreePath: real, Provisioning: domain.WorkspaceProvisioned}}

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.FileExists(t, filepath.Join(real, "README.md"))
}

func TestProjectDelete_NoCrowbarHome_SkipsDiskTeardown(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	uc := project.NewDelete(project.DeleteDeps{
		Projects: f.projects, Repos: f.repos, Workspaces: f.workspaces,
		RepoWorkspaces: f.cascade, Nodes: f.nodes,
		CrowbarHome: func() (string, error) { return "", errors.New("no home") },
	})
	require.NoError(t, uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}
