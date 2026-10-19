package exec_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	r := exec.Git(ctx, dir, "init", "-b", "main")
	require.Equal(t, 0, r.ExitCode)
	_ = exec.Git(ctx, dir, "config", "user.email", "test@test.com")
	_ = exec.Git(ctx, dir, "config", "user.name", "Test")
	return dir
}

func TestGit_Success(t *testing.T) {
	dir := initRepo(t)
	r := exec.Git(context.Background(), dir, "status")
	assert.Equal(t, 0, r.ExitCode)
	assert.Contains(t, r.Stdout, "branch")
}

func TestGit_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	r := exec.Git(context.Background(), dir, "log")
	assert.NotEqual(t, 0, r.ExitCode)
	assert.NotEmpty(t, r.Stderr)
}

func TestGitWithStdin_AppliesPatch(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()
	path := dir + "/hello.txt"
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0o600))
	_ = exec.Git(ctx, dir, "add", "hello.txt")
	_ = exec.Git(ctx, dir, "commit", "-m", "init")

	require.NoError(t, os.WriteFile(path, []byte("hello world\n"), 0o600))

	diff := exec.Git(ctx, dir, "diff")
	require.Equal(t, 0, diff.ExitCode)
	require.NotEmpty(t, diff.Stdout)

	r := exec.GitWithStdin(ctx, dir, diff.Stdout, "apply", "--cached")
	assert.Equal(t, 0, r.ExitCode, r.Stderr)
}

func TestRequireSuccess_ZeroExit(t *testing.T) {
	r := exec.Result{ExitCode: 0}
	assert.NoError(t, exec.RequireSuccess("op", r))
}

func TestRequireSuccess_NonZeroExit(t *testing.T) {
	r := exec.Result{ExitCode: 1, Stderr: "fatal: not a git repo"}
	err := exec.RequireSuccess("status", r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status")
	assert.Contains(t, err.Error(), "not a git repo")
}

func TestRequireSuccess_NonZeroExit_EmptyStderr_FallsBackToStdout(
	t *testing.T,
) {
	r := exec.Result{ExitCode: 1, Stderr: "", Stdout: "some stdout output"}
	err := exec.RequireSuccess("op", r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some stdout output")
}

func TestGit_ProcessCantStart_ExitCodeOne(
	t *testing.T,
) {
	ctx := context.Background()
	r := exec.Git(ctx, "/nonexistent/dir/path/xyz", "status")
	assert.Equal(t, 1, r.ExitCode)
}

// holdRefLocksHook is a reference-transaction hook that, once git holds the ref
// locks, reports on ready and then blocks on release; it gives up its stdout and
// stderr so a git that dies under it is not kept waiting on the pipes.
const holdRefLocksHook = `#!/bin/sh
[ "$1" = prepared ] || exit 0
exec >/dev/null 2>&1
echo held > '%READY%'
read _ < '%RELEASE%'
`

// unblockFIFO frees whoever still waits on the other end of path: a reader
// gets EOF. With nobody waiting the open fails and there is nothing to free.
func unblockFIFO(
	t *testing.T,
	path string,
) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err == nil {
		assert.NoError(t, f.Close())
	}
}

func lockFiles(
	t *testing.T,
	gitDir string,
) []string {
	t.Helper()
	var locks []string
	require.NoError(t, filepath.WalkDir(gitDir, func(path string, _ fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".lock") {
			locks = append(locks, path)
		}
		return err
	}))
	return locks
}

// A fetch cancelled while it holds its ref locks must get a signal git cleans
// up on (SIGTERM), not a SIGKILL that strands refs/...lock for every later fetch.
func TestGit_CancelledFetchLeavesNoRefLock(t *testing.T) {
	ctx := context.Background()
	src := initRepo(t)
	require.Equal(t, 0, exec.Git(ctx, src, "commit", "--allow-empty", "-m", "init").ExitCode)
	dst := initRepo(t)

	fifos := t.TempDir()
	ready, release := filepath.Join(fifos, "ready"), filepath.Join(fifos, "release")
	require.NoError(t, syscall.Mkfifo(ready, 0o600))
	require.NoError(t, syscall.Mkfifo(release, 0o600))
	t.Cleanup(func() { unblockFIFO(t, ready); unblockFIFO(t, release) })
	hooks := t.TempDir()
	hook := strings.NewReplacer("%READY%", ready, "%RELEASE%", release).Replace(holdRefLocksHook)
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "reference-transaction"), []byte(hook), 0o755))
	require.Equal(t, 0, exec.Git(ctx, dst, "config", "core.hooksPath", hooks).ExitCode)

	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	fetched := make(chan exec.Result, 1)
	go func() { fetched <- exec.Git(fetchCtx, dst, "fetch", src, "main:refs/remotes/origin/main") }()
	held := make(chan error, 1)
	go func() { _, err := os.ReadFile(ready); held <- err }()
	select {
	case err := <-held:
		require.NoError(t, err)
	case r := <-fetched:
		require.FailNow(t, "the fetch ended before its hook held the ref locks", "%+v", r)
	}
	gitDir := filepath.Join(dst, ".git")
	require.NotEmpty(t, lockFiles(t, gitDir), "the cancel must land while git holds a ref lock")

	cancel()
	r := <-fetched
	assert.NotEqual(t, 0, r.ExitCode)
	assert.Empty(t, lockFiles(t, gitDir), "a cancelled git removes its own lockfiles")
}
