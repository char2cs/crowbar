package stash_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/stash"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	r, err := exec.Git(ctx, dir, "init", "-b", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_, _ = exec.Git(ctx, dir, "config", "user.email", "test@test.com")
	_, _ = exec.Git(ctx, dir, "config", "user.name", "Test")
	return dir
}

func makeCommit(
	t *testing.T,
	dir string,
	filename string,
	content string,
	message string,
) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	_, _ = exec.Git(ctx, dir, "add", filename)
	r, err := exec.Git(ctx, dir, "commit", "-m", message)
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
}

func dirtyFile(
	t *testing.T,
	dir string,
	filename string,
	content string,
) {
	t.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestList_Empty(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	ss, err := stash.List(ctx, dir)

	require.NoError(t, err)
	assert.Empty(t, ss)
}

func TestPush_CreatesEntry(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")
	dirtyFile(t, dir, "file.txt", "dirty\n")

	err := stash.Push(ctx, dir, "my stash")

	require.NoError(t, err)

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	assert.Equal(t, "stash@{0}", ss[0].ID)
	assert.Contains(t, ss[0].Message, "my stash")
}

func TestPush_NoMessage(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")
	dirtyFile(t, dir, "file.txt", "dirty\n")

	err := stash.Push(ctx, dir, "")

	require.NoError(t, err)

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	assert.Equal(t, "stash@{0}", ss[0].ID)
}

func TestList_FilesChanged(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "a.txt", "aaa\n", "initial")
	makeCommit(t, dir, "b.txt", "bbb\n", "second")
	dirtyFile(t, dir, "a.txt", "aaa-modified\n")
	dirtyFile(t, dir, "b.txt", "bbb-modified\n")

	err := stash.Push(ctx, dir, "two files")
	require.NoError(t, err)

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	assert.Equal(t, 2, ss[0].FilesChanged)
}

func TestList_Date(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")
	dirtyFile(t, dir, "file.txt", "dirty\n")

	err := stash.Push(ctx, dir, "dated stash")
	require.NoError(t, err)

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	assert.False(t, ss[0].Date.IsZero())
}

func TestApply(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "original\n", "initial")
	dirtyFile(t, dir, "file.txt", "modified\n")

	err := stash.Push(ctx, dir, "apply-test")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(content))

	err = stash.Apply(ctx, dir, "stash@{0}")

	require.NoError(t, err)

	content, err = os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "modified\n", string(content))

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	assert.Len(t, ss, 1)
}

func TestPop(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "original\n", "initial")
	dirtyFile(t, dir, "file.txt", "modified\n")

	err := stash.Push(ctx, dir, "pop-test")
	require.NoError(t, err)

	err = stash.Pop(ctx, dir, "stash@{0}")

	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "modified\n", string(content))

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, ss)
}

func TestDrop(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "original\n", "initial")
	dirtyFile(t, dir, "file.txt", "modified\n")

	err := stash.Push(ctx, dir, "drop-test")
	require.NoError(t, err)

	err = stash.Drop(ctx, dir, "stash@{0}")

	require.NoError(t, err)

	ss, err := stash.List(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, ss)

	content, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(content))
}

func TestList_MultipleStashes(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "v1\n", "initial")

	dirtyFile(t, dir, "file.txt", "v2\n")
	err := stash.Push(ctx, dir, "first stash")
	require.NoError(t, err)

	dirtyFile(t, dir, "file.txt", "v3\n")
	err = stash.Push(ctx, dir, "second stash")
	require.NoError(t, err)

	ss, err := stash.List(ctx, dir)

	require.NoError(t, err)
	require.Len(t, ss, 2)
	assert.Equal(t, "stash@{0}", ss[0].ID)
	assert.Equal(t, "stash@{1}", ss[1].ID)
}

// TestList_ExecError exercises the exec.Git error path in List by passing a
// directory that does not exist (git cannot chdir into it).
func TestList_ExecError(
	t *testing.T,
) {
	ctx := context.Background()
	_, err := stash.List(ctx, "/nonexistent/path/that/does/not/exist")
	require.Error(t, err)
}

// TestPush_ExecError exercises the exec.Git error path in Push.
func TestPush_ExecError(
	t *testing.T,
) {
	ctx := context.Background()
	err := stash.Push(ctx, "/nonexistent/path/that/does/not/exist", "msg")
	require.Error(t, err)
}

// TestPush_RequireSuccessError exercises the RequireSuccess error path in Push.
// A repo that has no initial commit causes "git stash push" to exit non-zero.
func TestPush_RequireSuccessError(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	// Write a file but do NOT make any commit, then stage it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "file.txt")

	err := stash.Push(ctx, dir, "test")
	require.Error(t, err)
}

// TestApply_ExecError exercises the exec.Git error path in Apply.
func TestApply_ExecError(
	t *testing.T,
) {
	ctx := context.Background()
	err := stash.Apply(ctx, "/nonexistent/path/that/does/not/exist", "stash@{0}")
	require.Error(t, err)
}

// TestApply_RequireSuccessError exercises the RequireSuccess error path in Apply.
func TestApply_RequireSuccessError(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := stash.Apply(ctx, dir, "stash@{99}")
	require.Error(t, err)
}

// TestPop_ExecError exercises the exec.Git error path in Pop.
func TestPop_ExecError(
	t *testing.T,
) {
	ctx := context.Background()
	err := stash.Pop(ctx, "/nonexistent/path/that/does/not/exist", "stash@{0}")
	require.Error(t, err)
}

// TestPop_RequireSuccessError exercises the RequireSuccess error path in Pop.
func TestPop_RequireSuccessError(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := stash.Pop(ctx, dir, "stash@{99}")
	require.Error(t, err)
}

// TestDrop_ExecError exercises the exec.Git error path in Drop.
func TestDrop_ExecError(
	t *testing.T,
) {
	ctx := context.Background()
	err := stash.Drop(ctx, "/nonexistent/path/that/does/not/exist", "stash@{0}")
	require.Error(t, err)
}

// TestDrop_RequireSuccessError exercises the RequireSuccess error path in Drop.
func TestDrop_RequireSuccessError(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := stash.Drop(ctx, dir, "stash@{99}")
	require.Error(t, err)
}

func TestList_InjectedExecError(
	t *testing.T,
) {
	stash.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	_, err := stash.List(ctx, t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stash: list")
}

func TestPush_InjectedExecError(
	t *testing.T,
) {
	stash.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := stash.Push(ctx, t.TempDir(), "msg")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stash: push")
}

func TestApply_InjectedExecError(
	t *testing.T,
) {
	stash.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := stash.Apply(ctx, t.TempDir(), "stash@{0}")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stash: apply")
}

func TestPop_InjectedExecError(
	t *testing.T,
) {
	stash.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := stash.Pop(ctx, t.TempDir(), "stash@{0}")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stash: pop")
}

func TestDrop_InjectedExecError(
	t *testing.T,
) {
	stash.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := stash.Drop(ctx, t.TempDir(), "stash@{0}")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stash: drop")
}
