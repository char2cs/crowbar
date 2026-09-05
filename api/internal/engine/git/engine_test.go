package git_test

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func gitRun(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "Test User")
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
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	gitRun(t, dir, "add", filename)
	gitRun(t, dir, "commit", "-m", message)
}

func headSHA(
	t *testing.T,
	dir string,
) string {
	t.Helper()
	return gitOutput(t, dir, "rev-parse", "HEAD")
}

func gitOutput(
	t *testing.T,
	dir string,
	args ...string,
) string {
	t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestNew_ReturnsEngine(
	t *testing.T,
) {
	e := git.New()
	assert.NotNil(t, e)
}

func TestStatus_CleanRepo(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	status, err := e.Status(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, "main", status.Branch)
	assert.Empty(t, status.Files)
}

func TestWorkingTreeSummary_FromForkPoint(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	makeCommit(t, dir, "base.txt", "base\n", "fork commit")
	fork := headSHA(t, dir)

	makeCommit(t, dir, "new.txt", "new\n", "after fork")

	e := git.New()
	added, deleted, hasConflicts, hasCommits, err := e.WorkingTreeSummary(ctx, dir, fork)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, added, 0)
	assert.GreaterOrEqual(t, deleted, 0)
	assert.False(t, hasConflicts)
	assert.True(t, hasCommits)
}

func TestWorkingTreeSummary_EmptyForkPoint(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	added, deleted, hasConflicts, hasCommits, err := e.WorkingTreeSummary(ctx, dir, "")

	require.NoError(t, err)
	assert.GreaterOrEqual(t, added, 0)
	assert.GreaterOrEqual(t, deleted, 0)
	assert.False(t, hasConflicts)
	assert.True(t, hasCommits)
}

func TestOperationContinue_NoOperation(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.OperationContinue(ctx, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no in-progress operation")
}

func TestOperationAbort_NoOperation(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.OperationAbort(ctx, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no in-progress operation")
}

func TestOperationAbort_MergeConflict(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	makeCommit(t, dir, "conflict.txt", "line1\nline2\nline3\n", "base")
	gitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nFEATURE\nline3\n"), 0o600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "feature")

	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nMAIN\nline3\n"), 0o600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "main change")

	cmd := osexec.Command("git", "merge", "feature")
	cmd.Dir = dir
	_ = cmd.Run()

	e := git.New()
	err := e.OperationAbort(ctx, dir)
	require.NoError(t, err)
}

func TestStageFile_ThenUnstage(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o600))

	e := git.New()
	err := e.StageFile(ctx, dir, "new.txt")
	require.NoError(t, err)

	status, err := e.Status(ctx, dir)
	require.NoError(t, err)
	hasStaged := false
	for _, f := range status.Files {
		if f.Staged {
			hasStaged = true
		}
	}
	assert.True(t, hasStaged)

	err = e.UnstageFile(ctx, dir, "new.txt")
	require.NoError(t, err)
}

func TestDiscard_UntrackedFile(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "init")

	newPath := filepath.Join(dir, "untracked.txt")
	require.NoError(t, os.WriteFile(newPath, []byte("untracked\n"), 0o600))

	e := git.New()
	err := e.Discard(ctx, dir, "untracked.txt")
	require.NoError(t, err)

	_, err = os.Stat(newPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDiscard_TrackedFile(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "tracked.txt", "original\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified\n"), 0o600))

	e := git.New()
	err := e.Discard(ctx, dir, "tracked.txt")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(data))
}

// TestDiscard_SkipsNonMatchingStatusEntriesBeforeMatch covers Discard's status
// scan when the target file is not the first entry git status reports: the
// loop must skip over every non-matching file rather than stopping (or acting)
// on the first one, and still correctly classify the target once reached.
func TestDiscard_SkipsNonMatchingStatusEntriesBeforeMatch(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "init")

	// "aaa-first.txt" sorts and is reported before "target.txt" in git status,
	// so Discard's range over st.Files must pass over it via `continue` before
	// reaching the actual target.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aaa-first.txt"), []byte("first\n"), 0o600))
	newPath := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(newPath, []byte("untracked\n"), 0o600))

	e := git.New()
	err := e.Discard(ctx, dir, "target.txt")
	require.NoError(t, err)

	_, err = os.Stat(newPath)
	assert.True(t, os.IsNotExist(err), "the actually-targeted file must be discarded")
	_, err = os.Stat(filepath.Join(dir, "aaa-first.txt"))
	assert.NoError(t, err, "a file that only precedes the target in the status list must be untouched")
}

func TestWorktreeList(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	entries, err := e.WorktreeList(ctx, dir)

	require.NoError(t, err)
	require.NotEmpty(t, entries)
	assert.True(
		t,
		strings.HasSuffix(entries[0].Path, filepath.Base(dir)),
		"expected path ending with %s, got %s",
		filepath.Base(dir),
		entries[0].Path,
	)
}

func TestLog_Empty(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "first commit")

	e := git.New()
	commits, err := e.Log(ctx, dir, 10, 0)

	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "first commit", commits[0].Message)
}

func TestBlame_Basic(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "line one\nline two\n", "initial commit")

	e := git.New()
	entries, err := e.Blame(ctx, dir, "file.txt")

	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, 1, entries[0].LineNumber)
}

func TestBranches_List(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	branches, err := e.Branches(ctx, dir)

	require.NoError(t, err)
	require.NotEmpty(t, branches)
	hasCurrent := false
	for _, b := range branches {
		if b.IsCurrent {
			hasCurrent = true
		}
	}
	assert.True(t, hasCurrent)
}

func TestCommit_WithBody(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "init.txt", "init\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("change\n"), 0o600))
	gitRun(t, dir, "add", "change.txt")

	e := git.New()
	err := e.Commit(ctx, dir, "subject line", "body description")
	require.NoError(t, err)

	commits, err := e.Log(ctx, dir, 1, 0)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "subject line", commits[0].Message)
}

func TestCreateBranch_AndSwitch(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.CreateBranch(ctx, dir, "feature-x", "", true)
	require.NoError(t, err)

	status, err := e.Status(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "feature-x", status.Branch)
}

func TestStashes_PushAndList(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0o600))

	e := git.New()
	err := e.StashPush(ctx, dir, "my stash")
	require.NoError(t, err)

	stashes, err := e.Stashes(ctx, dir)
	require.NoError(t, err)
	require.Len(t, stashes, 1)
	assert.Contains(t, stashes[0].Message, "my stash")
}

func osWriteFile(
	dir string,
	name string,
	content string,
) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
