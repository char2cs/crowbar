// Ref-watch tests for the LINKED-worktree layout every Crowbar-managed workspace
// actually has: .git is a gitlink FILE, so the ref paths live in a resolved gitdir
// and in the parent repo's common dir — never under <worktree>/.git/.
package watch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func git(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

// newLinkedWorktree builds a real repository with one commit and adds a real linked
// worktree to it, returning the worktree path and the repo's common git dir.
func newLinkedWorktree(
	t *testing.T,
) (worktree, commonDir string) {
	t.Helper()
	// git writes the gitlink with the fully resolved path, and macOS hands out temp
	// dirs under the /var -> /private/var symlink; canonicalise so the paths the test
	// builds are the paths git (and therefore the watcher) sees.
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o700))

	git(t, repo, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hi\n"), 0o600))
	git(t, repo, "add", "a.txt")
	git(t, repo, "commit", "-q", "-m", "init")

	worktree = filepath.Join(root, "wt")
	git(t, repo, "worktree", "add", "-q", "-b", "feature", worktree)
	return worktree, filepath.Join(repo, ".git")
}

func startWatcher(
	t *testing.T,
	repoPath string,
) *Watcher {
	t.Helper()
	w := NewWatcher("ws-refs", repoPath, "", &minimalGit{}, &recordingDispatcher{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
	return w
}

// In a linked worktree the five paths the old code built (<worktree>/.git/HEAD, refs,
// refs/heads, refs/remotes, packed-refs) do not exist — .git is a gitlink file — so
// every Add failed and the ref watches were dead. Start must register the RESOLVED
// paths instead: HEAD in the worktree's own gitdir, the ref tree in the common dir.
func TestStart_WatchesResolvedRefPathsOfLinkedWorktree(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	w := startWatcher(t, worktree)

	gitDir := w.gitDir()
	require.NotEqual(t, filepath.Join(worktree, ".git"), gitDir,
		"a linked worktree's git dir must resolve out of the gitlink file")
	require.Equal(t, commonDir, w.commonDir(),
		"the common dir must resolve to the parent repo's .git")

	watched := watchList(t, w)
	assert.Contains(t, watched, filepath.Join(gitDir, "HEAD"),
		"the worktree's own HEAD must be watched (branch switches)")
	assert.Contains(t, watched, filepath.Join(gitDir, "refs"),
		"the worktree's private ref dir must be watched")
	assert.Contains(t, watched, filepath.Join(commonDir, "refs"),
		"the shared ref tree must be watched")
	assert.Contains(t, watched, filepath.Join(commonDir, "refs", "heads"),
		"shared branch refs must be watched (a bare fetch touches no working-tree file)")

	assert.NotContains(t, watched, filepath.Join(worktree, ".git"),
		"the gitlink file is not a watchable ref path")
}

// The main-worktree layout (.git is a real directory) must keep watching the same
// paths as before: git dir and common dir coincide.
func TestStart_WatchesRefPathsOfMainWorktree(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")

	w := startWatcher(t, repo)

	gitDir := filepath.Join(repo, ".git")
	require.Equal(t, gitDir, w.gitDir())
	require.Equal(t, gitDir, w.commonDir(), "a main worktree has no commondir file")

	watched := watchList(t, w)
	assert.Contains(t, watched, filepath.Join(gitDir, "HEAD"))
	assert.Contains(t, watched, filepath.Join(gitDir, "refs"))
	assert.Contains(t, watched, filepath.Join(gitDir, "refs", "heads"))
}

// Ref events arriving from the resolved roots must be recognised as git internals:
// they schedule the recompute and are never emitted as file changes (their path is
// outside the workspace).
func TestLinkedWorktree_RefEventSchedulesRecomputeAndNeverEmitsFileChange(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-refs-evt", worktree, "", &minimalGit{}, rd)

	cases := []struct {
		name string
		path string
	}{
		{
			name: "shared branch ref (a fetch that touches no working-tree file)",
			path: filepath.Join(commonDir, "refs", "heads", "main"),
		},
		{
			name: "shared remote-tracking ref",
			path: filepath.Join(commonDir, "refs", "remotes", "origin", "main"),
		},
		{
			name: "shared packed-refs",
			path: filepath.Join(commonDir, "packed-refs"),
		},
		{
			name: "the worktree's own HEAD",
			path: filepath.Join(w.gitDir(), "HEAD"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, w.shouldIgnore(tc.path),
				"a git-internal path must never be reported as a workspace file change")

			w.mu.Lock()
			w.gitPending = false
			w.mu.Unlock()

			w.handleOne(context.Background(), fsnotify.Event{Name: tc.path, Op: fsnotify.Write})

			assert.True(t, gitRecomputePending(w), "a ref change must schedule the git recompute")
		})
	}
}

// An unrelated file inside the resolved git dir (the index, a commit message) is
// still git-internal — ignored as a file change — but must NOT trigger a recompute.
func TestLinkedWorktree_NonRefGitFileSchedulesNothing(t *testing.T) {
	worktree, _ := newLinkedWorktree(t)
	w := NewWatcher("ws-refs-noise", worktree, "", &minimalGit{}, &recordingDispatcher{})

	index := filepath.Join(w.gitDir(), "index")
	assert.True(t, w.shouldIgnore(index))

	w.handleOne(context.Background(), fsnotify.Event{Name: index, Op: fsnotify.Write})
	assert.False(t, gitRecomputePending(w), "index churn must not schedule a git recompute")
}

// A real working-tree file in the worktree stays a file change: the git-internal
// classification must not swallow the workspace's own files.
func TestLinkedWorktree_WorkingTreeFileIsNotGitInternal(t *testing.T) {
	worktree, _ := newLinkedWorktree(t)
	w := NewWatcher("ws-refs-file", worktree, "", &minimalGit{}, &recordingDispatcher{})

	assert.False(t, w.shouldIgnore(filepath.Join(worktree, "a.txt")))
	assert.False(t, w.isGitInternal(filepath.Join(worktree, "a.txt")))
}
