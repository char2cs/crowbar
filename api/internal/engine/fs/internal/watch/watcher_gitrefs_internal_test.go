package watch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

func newLinkedWorktree(
	t *testing.T,
) (worktree, commonDir string) {
	t.Helper()
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

func TestSpec_NamesResolvedRefPathsOfLinkedWorktree(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	w := NewWatcher("ws-refs", worktree, "", &minimalGit{}, &recordingDispatcher{})

	gitDir := w.gitDir()
	require.NotEqual(t, filepath.Join(worktree, ".git"), gitDir,
		"a linked worktree's git dir must resolve out of the gitlink file")
	require.Equal(t, commonDir, w.commonDir(),
		"the common dir must resolve to the parent repo's .git")

	spec := w.spec()
	assert.Equal(t, worktree, spec.tree)
	assert.ElementsMatch(t, []string{gitDir, commonDir}, spec.gitDirs,
		"the worktree's own git dir must be watched (HEAD is replaced by rename on a branch switch)")
	assert.ElementsMatch(t, []string{filepath.Join(gitDir, "refs"), filepath.Join(commonDir, "refs")}, spec.refTrees,
		"the private and the shared ref trees must be watched (a bare fetch touches no working-tree file)")
	assert.True(t, spec.skip(filepath.Join(worktree, "sub", ".git")), "a nested .git dir is never watched as a working tree")
}

func TestSpec_NamesRefPathsOfMainWorktree(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	w := NewWatcher("ws-refs", repo, "", &minimalGit{}, &recordingDispatcher{})

	gitDir := filepath.Join(repo, ".git")
	require.Equal(t, gitDir, w.gitDir())
	require.Equal(t, gitDir, w.commonDir(), "a main worktree has no commondir file")

	spec := w.spec()
	assert.Equal(t, []string{gitDir, gitDir}, spec.gitDirs)
	assert.Equal(t, []string{filepath.Join(gitDir, "refs"), filepath.Join(gitDir, "refs")}, spec.refTrees)
	assert.True(t, spec.skip(gitDir), "the working-tree walk leaves .git to the git-dir watches")
}

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

			w.handleOne(context.Background(), event{Name: tc.path, Op: opWrite})

			assert.True(t, gitRecomputePending(w), "a ref change must schedule the git recompute")
		})
	}
}

func TestLinkedWorktree_NonRefGitFileSchedulesNothing(t *testing.T) {
	worktree, _ := newLinkedWorktree(t)
	w := NewWatcher("ws-refs-noise", worktree, "", &minimalGit{}, &recordingDispatcher{})

	index := filepath.Join(w.gitDir(), "index")
	assert.True(t, w.shouldIgnore(index))

	w.handleOne(context.Background(), event{Name: index, Op: opWrite})
	assert.False(t, gitRecomputePending(w), "index churn must not schedule a git recompute")
}

func TestLinkedWorktree_WorkingTreeFileIsNotGitInternal(t *testing.T) {
	worktree, _ := newLinkedWorktree(t)
	w := NewWatcher("ws-refs-file", worktree, "", &minimalGit{}, &recordingDispatcher{})

	assert.False(t, w.shouldIgnore(filepath.Join(worktree, "a.txt")))
	assert.False(t, w.isGitInternal(filepath.Join(worktree, "a.txt")))
}

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

func TestRefMove_RemoteTrackingRefUpdateReResolvesBase(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	g := startResolveWatcher(t, worktree)

	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")

	g.expectResolve(t, "a remote-tracking ref update")
}

func TestRefMove_NestedBranchUpdateReResolvesBase(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	git(t, repo, "branch", "feat/x")
	g := startResolveWatcher(t, worktree)

	git(t, repo, "update-ref", "refs/heads/feat/x", "HEAD")

	g.expectResolve(t, "a nested branch ref update")
}

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

func TestMaybeHandleGitRef_HeadSuffix(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := event{
		Name: filepath.Join(dir, ".git", "HEAD"),
		Op:   opWrite,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.True(t, gitRecomputePending(w), "git recompute should be scheduled for .git/HEAD event")
	assert.Equal(t, 0, rd.count(), "fanOutGit must not run synchronously (debounced)")
}

func TestMaybeHandleGitRef_RefsPath(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := event{
		Name: filepath.Join(dir, ".git", "refs", "heads", "main"),
		Op:   opWrite,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.True(t, gitRecomputePending(w), "git recompute should be scheduled for .git/refs/ event")
	assert.Equal(t, 0, rd.count(), "fanOutGit must not run synchronously (debounced)")
}

func TestMaybeHandleGitRef_PackedRefs(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := event{
		Name: filepath.Join(dir, ".git", "packed-refs"),
		Op:   opWrite,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.True(t, gitRecomputePending(w), "git recompute should be scheduled for .git/packed-refs event")
	assert.Equal(t, 0, rd.count(), "fanOutGit must not run synchronously (debounced)")
}

func TestMaybeHandleGitRef_UnrelatedGitFile_NoFanOut(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := event{
		Name: filepath.Join(dir, ".git", "COMMIT_EDITMSG"),
		Op:   opWrite,
	}
	w.maybeHandleGitRef(context.Background(), evt)

	assert.False(t, gitRecomputePending(w), "no git recompute should be scheduled for unrelated .git files")
	assert.Equal(t, 0, rd.count(), "OnGitStatus must NOT be called for unrelated .git files")
}

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

func TestIsRewriteInProgress_LinkedWorktree(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "wt")
	require.NoError(t, os.MkdirAll(worktree, 0o700))
	realGitDir := filepath.Join(root, "common", ".git", "worktrees", "wt")
	require.NoError(t, os.MkdirAll(realGitDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+realGitDir+"\n"), 0o600))

	w := newBareWatcher(worktree, &recordingDispatcher{})
	assert.False(t, w.isRewriteInProgress(), "clean linked worktree")

	require.NoError(t, os.MkdirAll(filepath.Join(realGitDir, "rebase-merge"), 0o700))
	assert.True(t, w.isRewriteInProgress(),
		"a rebase in a linked worktree must be detected via the resolved git dir")
}

func TestRefMove_PrivateWorktreeRefUpdateReResolvesBase(t *testing.T) {
	worktree, _ := newLinkedWorktree(t)
	git(t, worktree, "update-ref", "refs/bisect/bad", "HEAD")
	g := startResolveWatcher(t, worktree)

	git(t, worktree, "update-ref", "refs/bisect/good", "HEAD")

	g.expectResolve(t, "a per-worktree ref update")
}

func TestRefMove_PackRefsReResolvesBase(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	g := startResolveWatcher(t, worktree)

	git(t, repo, "pack-refs", "--all")

	g.expectResolve(t, "pack-refs rewriting packed-refs")
}

func TestRefMove_MainWorktreeBranchUpdateReResolvesBase(t *testing.T) {
	_, commonDir := newLinkedWorktree(t)
	repo := filepath.Dir(commonDir)
	g := startResolveWatcher(t, repo)

	git(t, repo, "update-ref", "refs/heads/other", "HEAD")

	g.expectResolve(t, "a branch created in a main worktree")
}
