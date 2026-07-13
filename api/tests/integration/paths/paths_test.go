//go:build integration

// Package paths_test is the friendly-worktree-path slice of the Task 17
// crash/recovery/rebuild/friendly-path integration matrix (spec §5 table, row
// "Friendly worktree path"; §3.9, decision 13). It asserts, through the real
// HTTP+WS+SQLite stack, that provisioned worktrees land at the human-readable
// <home>/projects/<project>/<host>/<owner>/<repo>/<branch>/ path — full remote
// slug on disk, UUIDs banished from navigable paths — and that a case-only
// collision is rejected while two repos differing only by host resolve to
// distinct, non-clashing paths.
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

// wsBranchInList reports whether a non-deleted workspace on the given branch
// exists in the repo's read-model list.
func wsBranchInList(t *testing.T, env *kit.Env, projectID, repoID, branch string) bool {
	t.Helper()
	resp := env.GET(t, "/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces")
	kit.RequireStatus(t, resp, http.StatusOK)
	var list []map[string]any
	kit.DecodeEnvData(t, resp, &list)
	for _, w := range list {
		if b, _ := w["branch"].(string); b == branch {
			if s, _ := w["status"].(string); s != "deleted" {
				return true
			}
		}
	}
	return false
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

// TestPaths_CaseOnlyClashRejected proves a git-distinct branch whose derived
// worktree path is case-insensitively equal to an existing sibling is REJECTED
// at creation on a case-insensitive filesystem (spec §5 "case-only clash
// rejected"; §3.9 residual case 1, decision 13 — reject, never disambiguate).
// The create is async and the clash predates any workspace id, so the rejection
// is observable as the absence of the second workspace (the provisioner fails
// before persisting a row; spec crud.go create-error path).
func TestPaths_CaseOnlyClashRejected(t *testing.T) {
	env := kit.BuildEnv(t)
	if !caseInsensitiveFS(t, env.HomeDir()) {
		t.Skip("case-sensitive filesystem: case-only paths do not collide on disk")
	}
	imported := env.ImportRepo(t, "clash", "")

	// First branch succeeds and provisions a worktree at .../feature-Case.
	env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature-Case")
	require.True(t, kit.DirExists(t, friendlyWorktree(env, imported.ProjectID, imported.RepoPath, "feature-Case")))

	// A case-only-different branch: branchTaken is case-sensitive (distinct
	// branch), so the request is accepted (202) and fails asynchronously in the
	// provisioner's DetectClash — no row is ever produced.
	resp := env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces",
		map[string]any{"branch": "feature-case"})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// The clashing workspace must never appear in the read model. Watching for 3
	// seconds and concluding "it never showed up" proves nothing — it only says the
	// row had not appeared YET, and on a loaded machine a slow provisioner would make
	// the test pass for entirely the wrong reason.
	//
	// The deterministic version of a NEVER is a barrier plus a plain assertion: run
	// the async create to COMPLETION (QuiesceReactors: the command dispatched, every
	// projection folded, every post-commit reactor joined), at which point the
	// provisioner has already failed in DetectClash and there is no longer anything
	// in flight that could produce the row. Its absence then is final, not merely
	// not-yet.
	env.QuiesceReactors()
	require.False(t, wsBranchInList(t, env, imported.ProjectID, imported.RepoID, "feature-case"),
		"case-only-clashing workspace must be rejected, never persisted")
}
