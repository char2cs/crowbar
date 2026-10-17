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
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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
	// Whether `git worktree add` creates the private refs/ dir up front depends on
	// the git version (2.43 does not); a per-worktree ref makes it exist on all.
	git(t, worktree, "update-ref", "refs/bisect/bad", "HEAD")
	w := startWatcher(t, worktree)

	gitDir := w.gitDir()
	require.NotEqual(t, filepath.Join(worktree, ".git"), gitDir,
		"a linked worktree's git dir must resolve out of the gitlink file")
	require.Equal(t, commonDir, w.commonDir(),
		"the common dir must resolve to the parent repo's .git")

	watched := watchList(t, w)
	assert.Contains(t, watched, gitDir,
		"the worktree's own git dir must be watched (HEAD is replaced by rename on a branch switch)")
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
	assert.Contains(t, watched, gitDir)
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

// resolveSignalGit reports every diff-base resolution, so a real-filesystem test
// can wait on "the watcher noticed a ref move and re-derived the base".
type resolveSignalGit struct {
	minimalGit
	resolved chan struct{}
}

func (g *resolveSignalGit) ResolveDiffBase(
	_ context.Context,
	_ string,
	_ string,
) gitdomain.DiffBase {
	select {
	case g.resolved <- struct{}{}:
	default:
	}
	return gitdomain.DiffBase{}
}

// startResolveWatcher starts a watcher on worktree and primes its base cache with
// one working-tree burst, so any later resolution is caused by a ref move.
func startResolveWatcher(
	t *testing.T,
	worktree string,
) *resolveSignalGit {
	t.Helper()
	g := &resolveSignalGit{resolved: make(chan struct{}, 16)}
	w := NewWatcher("ws-resolve", worktree, "main", g, &recordingDispatcher{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	require.NoError(t, os.WriteFile(filepath.Join(worktree, "prime.txt"), []byte("x"), 0o600))
	g.expectResolve(t, "the priming working-tree burst")
	return g
}

func (g *resolveSignalGit) expectResolve(
	t *testing.T,
	after string,
) {
	t.Helper()
	select {
	case <-g.resolved:
	case <-time.After(15 * time.Second):
		t.Fatalf("the diff base was not re-resolved after %s: the watcher never saw the ref move", after)
	}
}

// A fetch rewrites refs/remotes/<remote>/<branch>, a directory below the one
// Start adds; the cached base must still be dropped.
func TestRefMove_RemoteTrackingRefUpdateReResolvesBase(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	g := startResolveWatcher(t, worktree)

	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")

	g.expectResolve(t, "a remote-tracking ref update")
}

// A branch named feat/x lives in refs/heads/feat/, again below a watched root.
func TestRefMove_NestedBranchUpdateReResolvesBase(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	git(t, repo, "branch", "feat/x")
	g := startResolveWatcher(t, worktree)

	git(t, repo, "update-ref", "refs/heads/feat/x", "HEAD")

	g.expectResolve(t, "a nested branch ref update")
}

// git replaces HEAD by rename, so a watch on the HEAD file itself dies with the
// first change; every later checkout must still be seen.
func TestRefMove_RepeatedHeadChangesAllReResolveBase(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "second")
	g := startResolveWatcher(t, worktree)

	for _, rev := range []string{"main", "main~1", "main", "main~1"} {
		git(t, worktree, "checkout", "-q", "--detach", rev)
		g.expectResolve(t, "checkout --detach "+rev)
	}
}

// ---------------------------------------------------------------------------
// maybeHandleGitRef — direct invocation
// ---------------------------------------------------------------------------

func TestMaybeHandleGitRef_HeadSuffix(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	// Build an event whose Name has the .git/HEAD suffix relative to dir.
	evt := fsnotify.Event{
		Name: filepath.Join(dir, ".git", "HEAD"),
		Op:   fsnotify.Write,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	// The recompute is debounced: it is scheduled, never run synchronously.
	assert.True(t, gitRecomputePending(w), "git recompute should be scheduled for .git/HEAD event")
	assert.Equal(t, 0, rd.count(), "fanOutGit must not run synchronously (debounced)")
}

func TestMaybeHandleGitRef_RefsPath(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := fsnotify.Event{
		Name: filepath.Join(dir, ".git", "refs", "heads", "main"),
		Op:   fsnotify.Write,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.True(t, gitRecomputePending(w), "git recompute should be scheduled for .git/refs/ event")
	assert.Equal(t, 0, rd.count(), "fanOutGit must not run synchronously (debounced)")
}

// A change to .git/packed-refs (where refs land after git gc / pack-refs) must
// trigger the same GitStatus + SyncWorkingTreeState recompute as .git/HEAD.
func TestMaybeHandleGitRef_PackedRefs(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := fsnotify.Event{
		Name: filepath.Join(dir, ".git", "packed-refs"),
		Op:   fsnotify.Write,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.True(t, gitRecomputePending(w), "git recompute should be scheduled for .git/packed-refs event")
	assert.Equal(t, 0, rd.count(), "fanOutGit must not run synchronously (debounced)")
}

func TestMaybeHandleGitRef_UnrelatedGitFile_NoFanOut(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := fsnotify.Event{
		Name: filepath.Join(dir, ".git", "COMMIT_EDITMSG"),
		Op:   fsnotify.Write,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.False(t, gitRecomputePending(w), "no git recompute should be scheduled for unrelated .git files")
	assert.Equal(t, 0, rd.count(), "OnGitStatus must NOT be called for unrelated .git files")
}

// ---------------------------------------------------------------------------
// isRewriteInProgress — all three sentinel files
// ---------------------------------------------------------------------------

func TestIsRewriteInProgress_MergeHead(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))

	w := newBareWatcher(dir, &recordingDispatcher{})
	assert.False(t, w.isRewriteInProgress(), "should be false before creating MERGE_HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte("abc\n"), 0o600))
	assert.True(t, w.isRewriteInProgress(), "should be true with MERGE_HEAD present")
}

func TestIsRewriteInProgress_RebaseMerge(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0o700))

	w := newBareWatcher(dir, &recordingDispatcher{})
	assert.True(t, w.isRewriteInProgress())
}

func TestIsRewriteInProgress_RebaseApply(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "rebase-apply"), 0o700))

	w := newBareWatcher(dir, &recordingDispatcher{})
	assert.True(t, w.isRewriteInProgress())
}

func TestIsRewriteInProgress_None(t *testing.T) {
	dir := t.TempDir()
	w := newBareWatcher(dir, &recordingDispatcher{})
	assert.False(t, w.isRewriteInProgress())
}

// TestIsRewriteInProgress_LinkedWorktree proves H20: a child (linked) worktree's
// .git is a FILE ("gitdir: <path>") whose rebase/merge markers live in the
// per-worktree dir under the common dir, NOT in <worktree>/.git/. The guard must
// resolve the real git dir, or it reads "false" mid-rebase and broadcasts
// transient wrong status during the exact rewrite it should skip.
func TestIsRewriteInProgress_LinkedWorktree(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	require.NoError(t, os.MkdirAll(worktree, 0o700))
	realGitDir := filepath.Join(root, "common", ".git", "worktrees", "wt")
	require.NoError(t, os.MkdirAll(realGitDir, 0o700))
	// .git is a gitlink file pointing at the per-worktree git dir.
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+realGitDir+"\n"), 0o600))

	w := newBareWatcher(worktree, &recordingDispatcher{})
	assert.False(t, w.isRewriteInProgress(), "clean linked worktree")

	// A rebase-merge in the REAL git dir must be detected. With the old code this
	// stayed false (os.Stat(<worktree>/.git/rebase-merge) -> ENOTDIR, .git a file).
	require.NoError(t, os.MkdirAll(filepath.Join(realGitDir, "rebase-merge"), 0o700))
	assert.True(t, w.isRewriteInProgress(),
		"a rebase in a linked worktree must be detected via the resolved git dir")
}
