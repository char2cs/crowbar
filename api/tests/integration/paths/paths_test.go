//go:build integration

package paths_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

func friendlyWorktree(env *kit.Env, projectID, repoPath, branch string) string {
	return filepath.Join(env.HomeDir(), "projects", projectID, filepath.Base(repoPath), branch, "worktree")
}

func caseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	lower := filepath.Join(dir, "casetest-probe")
	require.NoError(t, os.WriteFile(lower, []byte("x"), 0o644))
	t.Cleanup(func() { _ = os.Remove(lower) })
	_, err := os.Stat(filepath.Join(dir, "CASETEST-PROBE"))
	return err == nil
}

func worktreeByBranch(t *testing.T, env *kit.Env, projectID, repoID, branch string) (map[string]any, bool) {
	t.Helper()
	for _, w := range env.WorktreeChats(t, projectID, repoID) {
		if b, _ := w["branch"].(string); b == branch {
			return w, true
		}
	}
	return nil, false
}

func TestPaths_FriendlyWorktreePath(t *testing.T) {
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "friendly", "")

	const branch = "feature/deep/nested"
	wsID := env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, branch)

	want := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, want), "worktree must exist at the friendly derived path %s", want)
	require.True(t, kit.FileExists(t, filepath.Join(want, ".git")),
		"the friendly leaf must be a real git worktree (.git pointer present)")

	require.NotContains(t, want, wsID, "worktree path must not contain the workspace UUID")
	require.NotContains(t, want, imported.RepoID, "worktree path must not contain the repo UUID")
	require.True(t, kit.DirExists(t, filepath.Join(env.HomeDir(), "projects",
		imported.ProjectID, filepath.Base(imported.RepoPath), "feature", "deep")),
		"nested branch must map to nested directories")
}

func TestPaths_FullSlugOnDisk_DistinctHosts(t *testing.T) {
	env := kit.BuildEnv(t)

	ghRepo := kit.InitRepo(t)
	kit.GitRun(t, ghRepo, "remote", "add", "origin", "git@github.com:acme/app.git")
	gh := env.ImportRepo(t, "gh", ghRepo)
	ghWS := env.CreateWorkspace(t, gh.ProjectID, gh.RepoID, "feature/x")

	glRepo := kit.InitRepo(t)
	kit.GitRun(t, glRepo, "remote", "add", "origin", "https://gitlab.com/acme/app")
	gl := env.ImportRepo(t, "gl", glRepo)
	glWS := env.CreateWorkspace(t, gl.ProjectID, gl.RepoID, "feature/x")

	ghPath := filepath.Join(env.HomeDir(), "projects", gh.ProjectID, "github.com", "acme", "app", "feature", "x", "worktree")
	glPath := filepath.Join(env.HomeDir(), "projects", gl.ProjectID, "gitlab.com", "acme", "app", "feature", "x", "worktree")

	require.True(t, kit.DirExists(t, ghPath), "github.com repo worktree must land at %s", ghPath)
	require.True(t, kit.DirExists(t, glPath), "gitlab.com repo worktree must land at %s", glPath)
	require.NotEqual(t, ghPath, glPath, "same owner/repo on different hosts must be distinct on disk")
	require.Contains(t, ghPath, filepath.Join("github.com", "acme", "app"))
	require.Contains(t, glPath, filepath.Join("gitlab.com", "acme", "app"))
	require.NotEmpty(t, ghWS)
	require.NotEmpty(t, glWS)
}

func TestPaths_CaseOnlyClashIsDisambiguated(t *testing.T) {
	env := kit.BuildEnv(t)
	if !caseInsensitiveFS(t, env.HomeDir()) {
		t.Skip("case-sensitive filesystem: case-only paths do not collide on disk")
	}
	imported := env.ImportRepo(t, "clash", "")

	ws1ID, ws1ChatID := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature-Case", "")
	frozenPath := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, "feature-Case")
	require.True(t, kit.DirExists(t, frozenPath))

	resp := env.PATCH(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+ws1ChatID+"/branch",
		map[string]any{"branch": "unrelated"})
	kit.RequireStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	require.True(t, kit.DirExists(t, frozenPath), "rename must not move the frozen directory")
	require.Equal(t, "unrelated", kit.BranchName(t, env.WorktreePath(imported.ProjectID, imported.RepoID, ws1ID)))

	ws2ID, _ := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature-case", "")
	require.NotEqual(t, ws1ID, ws2ID)

	wantSuffixed := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, "feature-case-2")
	gotPath := env.WorktreePath(imported.ProjectID, imported.RepoID, ws2ID)
	require.Equal(t, wantSuffixed, gotPath,
		"a directory clash with a frozen sibling must be disambiguated onto the next free suffix, never refused")
	require.True(t, kit.DirExists(t, gotPath))
	require.NotEqual(t, frozenPath, gotPath, "the two worktrees must never share a directory")

	require.Equal(t, "feature-case", kit.BranchName(t, gotPath),
		"the suffixed directory must still hold the real requested branch, not a renamed one")

	row, ok := worktreeByBranch(t, env, imported.ProjectID, imported.RepoID, "feature-case")
	require.True(t, ok, "the disambiguated worktree must still be addressable by its real branch")
	require.Equal(t, ws2ID, row["id"])
}
