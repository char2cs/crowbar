package exec_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

const lockFailureStderrFmt = "fatal: Unable to create '%s': File exists.\n\n" +
	"Another git process seems to be running in this repository"

func lockFailure(
	lockPath string,
) exec.Result {
	return exec.Result{
		ExitCode: 128,
		Stderr:   fmt.Sprintf(lockFailureStderrFmt, lockPath),
	}
}

func muteSleep(
	t *testing.T,
) {
	t.Helper()
	restore := exec.SetSleepForTest(func(time.Duration) {})
	t.Cleanup(restore)
}

func TestRunWithLockRetry_SucceedsAfterTransientLock(t *testing.T) {
	muteSleep(t)
	calls := 0
	r := exec.RunWithLockRetry(context.Background(), func() exec.Result {
		calls++
		if calls < 3 {
			return lockFailure("/repo/.git/index.lock")
		}
		return exec.Result{ExitCode: 0, Stdout: "ok"}
	})
	assert.Equal(t, 0, r.ExitCode)
	assert.Equal(t, 3, calls)
}

func TestRunWithLockRetry_NonLockFailureNotRetried(t *testing.T) {
	muteSleep(t)
	calls := 0
	r := exec.RunWithLockRetry(context.Background(), func() exec.Result {
		calls++
		return exec.Result{ExitCode: 1, Stderr: "fatal: not a git repository"}
	})
	assert.Equal(t, 1, r.ExitCode)
	assert.Equal(t, 1, calls)
}

func TestRunWithLockRetry_FreshLockNotDeleted(t *testing.T) {
	muteSleep(t)
	lock := filepath.Join(t.TempDir(), "index.lock")
	require.NoError(t, os.WriteFile(lock, nil, 0o600))

	calls := 0
	r := exec.RunWithLockRetry(context.Background(), func() exec.Result {
		calls++
		return lockFailure(lock)
	})

	assert.Equal(t, 128, r.ExitCode, "fresh lock must surface the failure")
	assert.Equal(t, 3, calls, "fresh lock must not earn the extra post-removal attempt")
	assert.FileExists(t, lock, "a fresh lock belongs to a live git process")
}

func TestRunWithLockRetry_StaleLockRemovedAndRetried(t *testing.T) {
	muteSleep(t)
	lock := filepath.Join(t.TempDir(), "index.lock")
	require.NoError(t, os.WriteFile(lock, nil, 0o600))
	restore := exec.SetNowForTest(func() time.Time {
		return time.Now().Add(time.Minute)
	})
	t.Cleanup(restore)

	calls := 0
	r := exec.RunWithLockRetry(context.Background(), func() exec.Result {
		calls++
		if _, err := os.Stat(lock); err == nil {
			return lockFailure(lock)
		}
		return exec.Result{ExitCode: 0}
	})

	assert.Equal(t, 0, r.ExitCode, "stale lock must be recovered")
	assert.Equal(t, 4, calls, "exactly one post-removal attempt")
	assert.NoFileExists(t, lock)
}

func TestRunWithLockRetry_CancelledContextStopsRetrying(t *testing.T) {
	muteSleep(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	r := exec.RunWithLockRetry(ctx, func() exec.Result {
		calls++
		return lockFailure("/repo/.git/index.lock")
	})

	assert.Equal(t, 128, r.ExitCode)
	assert.Equal(t, 1, calls)
}

func TestIsIndexLockFailure(t *testing.T) {
	assert.True(t, exec.IsIndexLockFailure(
		lockFailure("/r/.git/worktrees/wt/index.lock")))
	assert.False(t, exec.IsIndexLockFailure(exec.Result{
		ExitCode: 0,
		Stderr:   fmt.Sprintf(lockFailureStderrFmt, "/r/.git/index.lock"),
	}), "zero exit is never a lock failure")
	assert.False(t, exec.IsIndexLockFailure(exec.Result{
		ExitCode: 128,
		Stderr:   "fatal: Unable to create '/r/.git/HEAD.lock': File exists.",
	}), "only the index lock gets the retry treatment")
}

func TestLockPathFromResult(t *testing.T) {
	path := "/repo/.git/worktrees/wt-1/index.lock"
	assert.Equal(t, path, exec.LockPathFromResult(lockFailure(path)))
	assert.Empty(t, exec.LockPathFromResult(exec.Result{Stderr: "boom"}))
}

func TestRemoveStaleLock_MissingFile(t *testing.T) {
	assert.False(t, exec.RemoveStaleLock(""))
	assert.False(t, exec.RemoveStaleLock(filepath.Join(t.TempDir(), "nope.lock")))
}

// TestGit_StaleIndexLockRecovered drives the full path against real git: a
// stale index.lock (old mtime) must be cleared and the mutation succeed.
func TestGit_StaleIndexLockRecovered(t *testing.T) {
	muteSleep(t)
	dir := initRepo(t)
	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o600))

	lock := filepath.Join(dir, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lock, nil, 0o600))
	old := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(lock, old, old))

	r := exec.Git(ctx, dir, "add", "a.txt")
	assert.Equal(t, 0, r.ExitCode, r.Stderr)
	assert.NoFileExists(t, lock)
}

// TestGit_StatusDoesNotWriteIndex pins GIT_OPTIONAL_LOCKS=0: status with stale
// stat info must not rewrite the index (i.e. must not take index.lock), so
// watcher status reads can never collide with a user mutation.
func TestGit_StatusDoesNotWriteIndex(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	path := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o600))
	require.Equal(t, 0, exec.Git(ctx, dir, "add", "a.txt").ExitCode)
	require.Equal(t, 0, exec.Git(ctx, dir, "commit", "-m", "c").ExitCode)

	index := filepath.Join(dir, ".git", "index")
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
	before, err := os.Stat(index)
	require.NoError(t, err)

	require.Equal(t, 0, exec.Git(ctx, dir, "status", "--porcelain=v2").ExitCode)

	after, err := os.Stat(index)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime(),
		"status must not refresh the index opportunistically")
}
