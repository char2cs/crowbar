package exec_test

import (
	"context"
	"os"
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
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0600))
	_ = exec.Git(ctx, dir, "add", "hello.txt")
	_ = exec.Git(ctx, dir, "commit", "-m", "init")

	require.NoError(t, os.WriteFile(path, []byte("hello world\n"), 0600))

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
