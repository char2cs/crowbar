package git_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func errEngine() git.Engine {
	return git.NewWithErrorExec()
}

func TestStageFile_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().StageFile(ctx, t.TempDir(), "file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestUnstageFile_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().UnstageFile(ctx, t.TempDir(), "file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestDiscardTracked_ExecError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	err := errEngine().Discard(ctx, dir, "file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestDiscardUntracked_ExecError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o600))

	err := errEngine().Discard(ctx, dir, "new.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestDiscard_NotGitRepo_StatusError(t *testing.T) {
	ctx := context.Background()
	err := git.New().Discard(ctx, t.TempDir(), "file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: discard: status:")
}

func TestCommit_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Commit(ctx, t.TempDir(), "msg", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestPush_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Push(ctx, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestFetch_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Fetch(ctx, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestPull_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Pull(ctx, t.TempDir(), "merge")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestReset_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Reset(ctx, t.TempDir(), "mixed", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestMerge_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Merge(ctx, t.TempDir(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestRebase_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().Rebase(ctx, t.TempDir(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestWorktreeAdd_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().WorktreeAdd(ctx, t.TempDir(), "/tmp/wt", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestWorktreeRemove_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().WorktreeRemove(ctx, t.TempDir(), "/tmp/wt", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestWorktreeList_ExecError(t *testing.T) {
	ctx := context.Background()
	_, err := errEngine().WorktreeList(ctx, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestRebaseOnto_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().RebaseOnto(ctx, t.TempDir(), "new", "fork", "branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestMergeFFOnly_ExecError(t *testing.T) {
	ctx := context.Background()
	err := errEngine().MergeFFOnly(ctx, t.TempDir(), "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestOperationContinue_RebaseExecError(t *testing.T) {
	dir := initRepo(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git", "rebase-merge"), 0o700))

	err := errEngine().OperationContinue(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestOperationContinue_MergeExecError(t *testing.T) {
	dir := initRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "MERGE_HEAD"), []byte("abc123"), 0o600))

	err := errEngine().OperationContinue(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestOperationAbort_RebaseExecError(t *testing.T) {
	dir := initRepo(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git", "rebase-merge"), 0o700))

	err := errEngine().OperationAbort(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestOperationAbort_MergeExecError(t *testing.T) {
	dir := initRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "MERGE_HEAD"), []byte("abc123"), 0o600))

	err := errEngine().OperationAbort(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestOperationAbort_SquashExecError(t *testing.T) {
	dir := initRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "SQUASH_HEAD"), []byte("abc123"), 0o600))

	err := errEngine().OperationAbort(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec error")
}

func TestWorkingTreeSummary_NotGitRepo_ConflictsError(t *testing.T) {
	ctx := context.Background()
	_, _, _, _, err := git.New().WorkingTreeSummary(ctx, t.TempDir(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git: summary: conflicts:")
}

func TestWorkingTreeSummary_InvalidForkPoint_NumstatNonZeroExit(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	added, deleted, _, _, err := git.New().WorkingTreeSummary(ctx, dir, "deadbeef000000deadbeef000000deadbeef000000")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, deleted)
}

func TestWorkingTreeSummary_EmptyRepo_RevListNonZeroExit(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	_, _, _, hasCommits, err := git.New().WorkingTreeSummary(ctx, dir, "")
	require.NoError(t, err)
	assert.False(t, hasCommits)
}

func TestNumstatFromForkPoint_ExecError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	fork := headSHA(t, dir)

	added, deleted, _, _, err := errEngine().WorkingTreeSummary(ctx, dir, fork)
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, deleted)
}

func TestRevListHasCommits_ExecError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	_, _, _, hasCommits, err := errEngine().WorkingTreeSummary(ctx, dir, "")
	require.NoError(t, err)
	assert.False(t, hasCommits)
}

func TestParseNumstat_ShortLine_Skipped(t *testing.T) {
	added, deleted, err := git.ExportedParseNumstat("orphan\n")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, deleted)
}

func TestParseNumstat_NegativeValues_ClampedToZero(t *testing.T) {
	added, deleted, err := git.ExportedParseNumstat("-5\t-3\tfile.go\n")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, deleted)
}

func TestApplyHunk_ExecStdinError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "hello.go", "package main\n\nfunc hello() {}\n", "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc hello() { return }\n"), 0o600))

	files, err := git.New().Diff(ctx, dir, false)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	require.NotEmpty(t, files[0].Hunks)
	hunkID := files[0].Hunks[0].HunkID

	err = errEngine().StageHunk(ctx, dir, "hello.go", hunkID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected exec stdin error")
}

func TestCreateBranch_AlreadyExists_ReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	require.NoError(t, e.CreateBranch(ctx, dir, "dup-branch", "", false))

	err := e.CreateBranch(ctx, dir, "dup-branch", "", false)

	require.Error(t, err)
	assert.ErrorIs(t, err, git.ErrBranchAlreadyExists)
}

func TestSwitchBranch_UnknownName_ReturnsUnclassifiedError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.SwitchBranch(ctx, dir, "no-such-branch")

	require.Error(t, err)
	assert.NotErrorIs(t, err, git.ErrBranchNotFound, "exit-1 checkout failures are not classified as ErrBranchNotFound today")
}

func TestReclassifyError_NonGitErrorPassesThrough(t *testing.T) {
	boom := errors.New("boom")

	got := git.ExportedReclassifyError(boom)

	assert.Same(t, boom, got)
}

func TestFetchPrune_NoRemote_ReturnsError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.FetchPrune(ctx, dir)

	assert.Error(t, err, "a repo with no origin remote must fail FetchPrune, not hang or silently succeed")
}

func TestPush_OriginNotARepo_ReasonIsGitsStderr(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	gitRun(t, dir, "remote", "add", "origin", filepath.Join(t.TempDir(), "gone"))
	gitRun(t, dir, "config", "push.autoSetupRemote", "true")

	err := git.New().Push(ctx, dir)

	require.ErrorIs(t, err, git.ErrNoRemote)
	got := problem.Of(fmt.Errorf("git: mutate: %w", err))
	assert.Equal(t, problem.InvalidArgument, got.Code)
	assert.True(t, strings.HasPrefix(got.Message, "git: no_remote: fatal: "), got.Message)
	assert.Contains(t, got.Message, "does not appear to be a git repository")
	assert.NotContains(t, got.Message, "exit 128")
	assert.NotContains(t, got.Message, "mutate")
}

func TestWorktreeAddAtRef_PathTaken_ReasonIsGitsStderr(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	taken := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(taken, "x"), []byte("x"), 0o600))

	_, err := git.New().WorktreeAddAtRef(ctx, dir, taken, "feature", "HEAD")

	require.Error(t, err)
	got := problem.Of(fmt.Errorf("import chat: create workspace: create child: worktree checkout: %w", err))
	assert.Contains(t, got.Message, "fatal: '"+taken+"' already exists")
	assert.NotContains(t, got.Message, "import chat")
	assert.NotContains(t, got.Message, "exit 128")
}

func TestFastForwardBranch_NoRemote_ReturnsError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.FastForwardBranch(ctx, dir, "main")

	assert.Error(t, err, "a repo with no origin remote must fail FastForwardBranch, not hang or silently succeed")
}

func TestOperationInProgress_NoOperation(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	op, err := e.OperationInProgress(ctx, dir)

	require.NoError(t, err)
	assert.Empty(t, op)
}

func TestStageHunk_StaleHunkID_ReturnsErrStaleHunk(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0o600))

	e := git.New()
	err := e.StageHunk(ctx, dir, "file.txt", "not-a-real-hunk-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hunk not found in current diff")
}
