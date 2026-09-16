//go:build integration

// Package paths_test is the friendly-worktree-path slice of the Task 17
// crash/recovery/rebuild/friendly-path integration matrix (spec §5 table, row
// "Friendly worktree path"; §3.9, decision 13). It asserts, through the real
// HTTP+WS+SQLite stack, that provisioned worktrees land at the human-readable
// <home>/projects/<project>/<host>/<owner>/<repo>/<branch>/ path — full remote
// slug on disk, UUIDs banished from navigable paths — and that a case-only
// DIRECTORY collision is disambiguated (see TestPaths_CaseOnlyClashIsDisambiguated)
// while two repos differing only by host resolve to distinct, non-clashing paths.
package paths_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration entry point for the paths package.
func TestMain(m *testing.M) {
	kit.Main(m)
}

// friendlyWorktree mirrors the server-side worktreepath.Derive result for a
// no-remote local repo: <home>/projects/<project>/<repoName>/<branch>/worktree,
// where the slug degrades to the repo's on-disk name (filepath.Base(repoPath)),
// branch separators map to nested directories, and the trailing "worktree" leaf
// makes <repoName>/<branch> a workspace root sibling of "chats" (spec §3.5/§3.9).
// The kit deliberately does not export this (worktreePath is server-internal),
// so the matrix derives it here.
func friendlyWorktree(env *kit.Env, projectID, repoPath, branch string) string {
	return filepath.Join(env.HomeDir(), "projects", projectID, filepath.Base(repoPath), branch, "worktree")
}

// caseInsensitiveFS reports whether dir lives on a case-insensitive filesystem
// (macOS APFS / Windows), so the case-only-clash assertion runs only where the
// on-disk collision it guards against can actually occur.
func caseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	lower := filepath.Join(dir, "casetest-probe")
	require.NoError(t, os.WriteFile(lower, []byte("x"), 0o644))
	t.Cleanup(func() { _ = os.Remove(lower) })
	_, err := os.Stat(filepath.Join(dir, "CASETEST-PROBE"))
	return err == nil
}

// worktreeByBranch finds the repo's chat-owned worktree row on the given
// branch, via the chat-list read model that replaced the deleted workspace
// list (spec §8 step 6): every worktree-owning chat carries its git state
// inline, so this is the one read that answers what GET .../workspaces used
// to.
func worktreeByBranch(t *testing.T, env *kit.Env, projectID, repoID, branch string) (map[string]any, bool) {
	t.Helper()
	for _, w := range env.WorktreeChats(t, projectID, repoID) {
		if b, _ := w["branch"].(string); b == branch {
			return w, true
		}
	}
	return nil, false
}

// TestPaths_FriendlyWorktreePath proves a created workspace's git worktree lands
// at the human-readable derived path (spec §5 "Friendly worktree path", §3.9): a
// no-remote repo's slug is its on-disk name, a nested branch maps to nested
// directories, the leaf is a real worktree (.git present), and no UUID (the
// workspace or repo id) appears anywhere in the navigable path (decision 13).
func TestPaths_FriendlyWorktreePath(t *testing.T) {
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "friendly", "")

	const branch = "feature/deep/nested"
	wsID := env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, branch)

	want := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, want), "worktree must exist at the friendly derived path %s", want)
	// A linked worktree's .git is a gitdir-pointer FILE (not a directory).
	require.True(t, kit.FileExists(t, filepath.Join(want, ".git")),
		"the friendly leaf must be a real git worktree (.git pointer present)")

	// Navigable path carries no UUIDs: neither the workspace id nor the repo id
	// appears as a path segment (spec §3.9, decision 13).
	require.NotContains(t, want, wsID, "worktree path must not contain the workspace UUID")
	require.NotContains(t, want, imported.RepoID, "worktree path must not contain the repo UUID")
	// Branch separators nested into real directories.
	require.True(t, kit.DirExists(t, filepath.Join(env.HomeDir(), "projects",
		imported.ProjectID, filepath.Base(imported.RepoPath), "feature", "deep")),
		"nested branch must map to nested directories")
}

// TestPaths_FullSlugOnDisk_DistinctHosts proves the repo segment is encoded as
// its FULL remote slug host/owner/repo on disk, so two repos differing ONLY by
// host resolve to DISTINCT paths with no clash (spec §5 "Friendly worktree
// path": "two repos differing only by host resolve to distinct paths"; §3.9,
// decision 13, RATIFIED 2026-07-07 host-in-template). `git remote add` never
// contacts the host, so the github.com/gitlab.com slugs are exercised offline.
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
	// The full host/owner/repo slug (not a bare acme/app leaf) is what makes them distinct.
	require.Contains(t, ghPath, filepath.Join("github.com", "acme", "app"))
	require.Contains(t, glPath, filepath.Join("gitlab.com", "acme", "app"))
	require.NotEmpty(t, ghWS)
	require.NotEmpty(t, glWS)
}

// TestPaths_CaseOnlyClashIsDisambiguated proves a create whose derived
// worktree DIRECTORY is case-insensitively equal to an existing (frozen)
// sibling's lands on a disambiguated suffix instead of colliding on disk
// (worktreepath.FreePathBranch — "fix(workspace): freeze a workspace's
// directory at creation so a rename moves nothing", #132). That commit
// deliberately replaced decision 13's original "reject, never disambiguate"
// with "take the next free variant": a workspace directory is frozen at
// creation and never follows a later branch rename (hierarchy.RenameBranch:
// "the directory keeps its original name... the create path disambiguates a
// name a previous workspace has frozen"), so refusing a case-only-clashing
// name would permanently block a create on a name an unrelated, already
// -renamed-away workspace happens to still be squatting on disk.
//
// Two literally case-variant branches (feature-Case / feature-case) cannot
// coexist as git refs on a case-insensitive filesystem in the first place —
// `git worktree add -b` for the second fails with "a branch named ... already
// exists" before any worktree-path logic runs, because loose refs live under
// .git/refs on the SAME filesystem. So the fixture that actually exercises
// FreePathBranch's suffixing is the one #132's own doc comment describes: rename
// the first workspace's branch AWAY (freeing the git ref, but never moving its
// frozen directory — RenameBranch's whole point), then create a second,
// entirely fresh workspace whose branch is a case-only variant of that frozen
// directory name.
func TestPaths_CaseOnlyClashIsDisambiguated(t *testing.T) {
	env := kit.BuildEnv(t)
	if !caseInsensitiveFS(t, env.HomeDir()) {
		t.Skip("case-sensitive filesystem: case-only paths do not collide on disk")
	}
	imported := env.ImportRepo(t, "clash", "")

	// First workspace freezes the "feature-Case" directory.
	ws1ID, ws1ChatID := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature-Case", "")
	frozenPath := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, "feature-Case")
	require.True(t, kit.DirExists(t, frozenPath))

	// Rename it away: the git ref "feature-Case" is gone (freeing it for reuse
	// at the git level), but the directory stays frozen at .../feature-Case —
	// RenameBranch never moves it.
	resp := env.PATCH(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+ws1ChatID+"/branch",
		map[string]any{"branch": "unrelated"})
	kit.RequireStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	require.True(t, kit.DirExists(t, frozenPath), "rename must not move the frozen directory")
	require.Equal(t, "unrelated", kit.BranchName(t, env.WorktreePath(imported.ProjectID, imported.RepoID, ws1ID)))

	// A brand new workspace on "feature-case" is git-distinct from anything now
	// live (only "unrelated" exists where "feature-Case" once did), so the
	// create itself succeeds — but its derived directory collides
	// case-insensitively with the frozen "feature-Case", so FreePathBranch must
	// land it on the next free suffix instead of refusing the create.
	ws2ID, _ := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature-case", "")
	require.NotEqual(t, ws1ID, ws2ID)

	wantSuffixed := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, "feature-case-2")
	gotPath := env.WorktreePath(imported.ProjectID, imported.RepoID, ws2ID)
	require.Equal(t, wantSuffixed, gotPath,
		"a directory clash with a frozen sibling must be disambiguated onto the next free suffix, never refused")
	require.True(t, kit.DirExists(t, gotPath))
	require.NotEqual(t, frozenPath, gotPath, "the two worktrees must never share a directory")

	// The disambiguation is a DIRECTORY-naming device only: the worktree it
	// produced must still be checked out on the branch actually requested.
	require.Equal(t, "feature-case", kit.BranchName(t, gotPath),
		"the suffixed directory must still hold the real requested branch, not a renamed one")

	row, ok := worktreeByBranch(t, env, imported.ProjectID, imported.RepoID, "feature-case")
	require.True(t, ok, "the disambiguated worktree must still be addressable by its real branch")
	require.Equal(t, ws2ID, row["id"])
}
