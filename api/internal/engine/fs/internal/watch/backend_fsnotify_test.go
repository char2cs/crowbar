//go:build !darwin

package watch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func watchList(
	t *testing.T,
	be backend,
) []string {
	t.Helper()
	switch fb := be.(type) {
	case *fsnotifyBackend:
		return fb.fsw.WatchList()
	case *fileBackend:
		return fb.fsw.WatchList()
	}
	require.FailNow(t, "non-darwin builds use the fsnotify backends")
	return nil
}

func newTestBackend(
	t *testing.T,
	spec watchSpec,
) backend {
	t.Helper()
	be, err := newBackend(t.Context(), spec)
	require.NoError(t, err)
	t.Cleanup(func() { _ = be.close() })
	return be
}

func TestFsnotifyBackend_SkippedDirsAreNeverWatched(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))
	skip := func(p string) bool {
		base := filepath.Base(p)
		return base == "node_modules" || base == "dist"
	}
	be := newTestBackend(t, watchSpec{tree: dir, skip: skip})

	watched := watchList(t, be)
	assert.Contains(t, watched, dir, "repo root must be watched")
	assert.Contains(t, watched, filepath.Join(dir, "src"), "non-ignored dir must be watched")
	assert.NotContains(t, watched, filepath.Join(dir, "node_modules"), "a skipped directory must not be watched")
	assert.NotContains(t, watched, filepath.Join(dir, "node_modules", "pkg"), "nor its descendants")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "lib", "x"), 0o700))
	be.created(t.Context(), filepath.Join(dir, "dist"))
	be.created(t.Context(), filepath.Join(dir, "lib"))

	watched = watchList(t, be)
	assert.NotContains(t, watched, filepath.Join(dir, "dist"), "a skipped directory created later must not be watched")
	assert.Contains(t, watched, filepath.Join(dir, "lib", "x"), "a new directory is watched at every depth")
}

func TestFsnotifyBackend_WatchesResolvedGitDirsAndRefTrees(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	git(t, worktree, "update-ref", "refs/bisect/bad", "HEAD")
	w := NewWatcher("ws-refs", worktree, "", &minimalGit{}, &recordingDispatcher{})
	gitDir := w.gitDir()
	be := newTestBackend(t, w.spec())

	watched := watchList(t, be)
	assert.Contains(t, watched, gitDir, "HEAD is replaced by rename, so its directory is watched")
	assert.Contains(t, watched, filepath.Join(gitDir, "refs"), "the private ref dir")
	assert.Contains(t, watched, filepath.Join(commonDir, "refs"), "the shared ref tree")
	assert.Contains(t, watched, filepath.Join(commonDir, "refs", "heads"), "shared branch refs")
	assert.NotContains(t, watched, filepath.Join(worktree, ".git"), "the gitlink file")

	feat := filepath.Join(commonDir, "refs", "heads", "feat")
	rebase := filepath.Join(gitDir, "rebase-merge")
	require.NoError(t, os.MkdirAll(feat, 0o700))
	require.NoError(t, os.MkdirAll(rebase, 0o700))
	be.created(t.Context(), feat)
	be.created(t.Context(), rebase)

	watched = watchList(t, be)
	assert.Contains(t, watched, feat, "a new ref directory (branch feat/x) is followed")
	assert.NotContains(t, watched, rebase, "git-internal directories outside the ref trees are not")
}

func TestFsnotifyBackend_WalkStopsAtTheNextDirectoryOnceCancelled(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"a", "b", "c"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o700))
	}
	ctx, cancel := context.WithCancel(t.Context())
	var visited []string
	skip := func(p string) bool {
		visited = append(visited, p)
		cancel()
		return false
	}

	be, err := newBackend(ctx, watchSpec{tree: dir, skip: skip})

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, be)
	assert.Equal(t, []string{dir}, visited, "no directory is visited after the cancel")
}

func TestFsnotifyBackend_FileSpecWatchesOnlyTheParentDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deeper"), 0o700))
	be := newTestBackend(t, watchSpec{tree: filepath.Join(dir, "a.md"), file: true})

	require.NoError(t, os.Mkdir(filepath.Join(dir, "later"), 0o700))
	be.created(t.Context(), filepath.Join(dir, "later"))

	assert.Equal(t, []string{dir}, watchList(t, be))
}

func TestFileBackend_AnExhaustedInotifyLimitIsTheWatchLimit(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.EMFILE, syscall.ENFILE, syscall.ENOSPC} {
		err := watchLimit(fmt.Errorf("new fsnotify watcher: %w", errno))

		require.ErrorIs(t, err, ErrWatchLimit, errno.Error())
		require.ErrorIs(t, err, errno, "the cause is kept")
	}
	require.NotErrorIs(t, watchLimit(fs.ErrPermission), ErrWatchLimit)
	require.NoError(t, watchLimit(nil))
}
