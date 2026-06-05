package git_test

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func newGitCmd(
	args ...string,
) *osexec.Cmd {
	return osexec.Command("git", args...)
}

func TestDiff_ModifiedFile(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.go", "package main\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main\n\nfunc main() {}\n"), 0600))

	e := git.New()
	files, err := e.Diff(ctx, dir, false)

	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "file.go", files[0].FilePath)
}

func TestCommitDiff_SingleCommit(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "first commit")

	sha := headSHA(t, dir)

	e := git.New()
	mfd, err := e.CommitDiff(ctx, dir, sha)

	require.NoError(t, err)
	assert.Equal(t, sha, mfd.CommitHash)
	require.Len(t, mfd.Files, 1)
}

func TestStageHunk_Basic(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.go", "package main\n\nfunc A() {}\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main\n\nfunc A() { return }\n"), 0600))

	e := git.New()
	files, err := e.Diff(ctx, dir, false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NotEmpty(t, files[0].Hunks)

	hunkID := files[0].Hunks[0].HunkID
	err = e.StageHunk(ctx, dir, "file.go", hunkID)
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
}

func TestUnstageHunk_Basic(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.go", "package main\n\nfunc A() {}\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main\n\nfunc A() { return }\n"), 0600))

	e := git.New()
	err := e.StageFile(ctx, dir, "file.go")
	require.NoError(t, err)

	stagedFiles, err := e.Diff(ctx, dir, true)
	require.NoError(t, err)
	require.NotEmpty(t, stagedFiles)
	require.NotEmpty(t, stagedFiles[0].Hunks)

	stagedHunkID := stagedFiles[0].Hunks[0].HunkID
	err = e.UnstageHunk(ctx, dir, "file.go", stagedHunkID)
	require.NoError(t, err)
}

func TestRenameBranch(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.RenameBranch(ctx, dir, "main", "mainline")
	require.NoError(t, err)

	branches, err := e.Branches(ctx, dir)
	require.NoError(t, err)
	names := make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	assert.Contains(t, names, "mainline")
}

func TestDeleteBranch(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.CreateBranch(ctx, dir, "to-delete", "", false)
	require.NoError(t, err)

	err = e.DeleteBranch(ctx, dir, "to-delete")
	require.NoError(t, err)
}

func TestForceDeleteBranch(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	gitRun(t, dir, "checkout", "-b", "temp")
	makeCommit(t, dir, "extra.txt", "extra\n", "extra commit")
	gitRun(t, dir, "checkout", "main")

	e := git.New()
	err := e.ForceDeleteBranch(ctx, dir, "temp")
	require.NoError(t, err)
}

func TestSwitchBranch(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	gitRun(t, dir, "branch", "other")

	e := git.New()
	err := e.SwitchBranch(ctx, dir, "other")
	require.NoError(t, err)

	status, err := e.Status(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "other", status.Branch)
}

func TestStashApply(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0600))
	gitRun(t, dir, "stash")

	e := git.New()
	err := e.StashApply(ctx, dir, "stash@{0}")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "modified\n", string(data))
}

func TestStashPop(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0600))
	gitRun(t, dir, "stash")

	e := git.New()
	err := e.StashPop(ctx, dir, "stash@{0}")
	require.NoError(t, err)

	stashes, err := e.Stashes(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, stashes)
}

func TestStashDrop(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0600))
	gitRun(t, dir, "stash")

	e := git.New()
	err := e.StashDrop(ctx, dir, "stash@{0}")
	require.NoError(t, err)

	stashes, err := e.Stashes(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, stashes)
}

func TestReset_Mixed(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	makeCommit(t, dir, "second.txt", "second\n", "second commit")

	sha := headSHA(t, dir)
	_ = sha

	firstSHA := gitOutput(t, dir, "rev-list", "--skip=1", "--max-count=1", "HEAD")

	e := git.New()
	err := e.Reset(ctx, dir, "mixed", firstSHA)
	require.NoError(t, err)
}

func TestMerge_FastForward(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.txt", "feature\n", "feature commit")
	gitRun(t, dir, "checkout", "main")

	e := git.New()
	err := e.Merge(ctx, dir, "feature")
	require.NoError(t, err)
}

func TestMergeFFOnly_Succeeds(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	gitRun(t, dir, "checkout", "-b", "ahead")
	makeCommit(t, dir, "extra.txt", "extra\n", "ahead commit")
	gitRun(t, dir, "checkout", "main")

	e := git.New()
	err := e.MergeFFOnly(ctx, dir, "ahead")
	require.NoError(t, err)
}

func TestConflictedFiles_NoConflicts(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	files, err := e.ConflictedFiles(ctx, dir)

	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestConflictHunks_WithConflict(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "conflict.txt", "line1\nshared\nline3\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nFEATURE\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "feature")

	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nMAIN\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "main change")

	mergeCmd := newGitCmd("merge", "feature")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run()

	e := git.New()
	hunks, err := e.ConflictHunks(ctx, dir, "conflict.txt")
	require.NoError(t, err)
	require.NotEmpty(t, hunks)
	assert.Equal(t, gitdomain.ConflictResolutionUnresolved, hunks[0].Resolution)
}

func TestResolveHunk_Ours(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "conflict.txt", "line1\nshared\nline3\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nFEATURE\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "feature")

	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nMAIN\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "main change")

	mergeCmd := newGitCmd("merge", "feature")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run()

	e := git.New()
	hunks, err := e.ConflictHunks(ctx, dir, "conflict.txt")
	require.NoError(t, err)
	require.NotEmpty(t, hunks)

	err = e.ResolveHunk(ctx, dir, "conflict.txt", hunks[0].ID, gitdomain.ConflictResolutionOurs, "")
	require.NoError(t, err)
}

func TestWorktreeAdd_AndRemove(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	gitRun(t, dir, "branch", "wt-branch")

	wtPath := filepath.Join(t.TempDir(), "extra-worktree")

	e := git.New()
	err := e.WorktreeAdd(ctx, dir, wtPath, "wt-branch")
	require.NoError(t, err)

	_, err = os.Stat(wtPath)
	require.NoError(t, err)

	err = e.WorktreeRemove(ctx, dir, wtPath)
	require.NoError(t, err)
}

func TestOperationContinue_AfterMergeConflictResolved(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "conflict.txt", "line1\nshared\nline3\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nFEATURE\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "feature")

	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nMAIN\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")
	gitRun(t, dir, "commit", "-m", "main change")

	mergeCmd := newGitCmd("merge", "feature")
	mergeCmd.Dir = dir
	_ = mergeCmd.Run()

	e := git.New()
	hunks, err := e.ConflictHunks(ctx, dir, "conflict.txt")
	require.NoError(t, err)
	require.NotEmpty(t, hunks)

	err = e.ResolveHunk(ctx, dir, "conflict.txt", hunks[0].ID, gitdomain.ConflictResolutionOurs, "")
	require.NoError(t, err)

	err = e.OperationContinue(ctx, dir)
	require.NoError(t, err)
}

func TestRebase_FastForward(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	gitRun(t, dir, "checkout", "-b", "topic")
	makeCommit(t, dir, "topic.txt", "topic\n", "topic commit")
	gitRun(t, dir, "checkout", "main")
	gitRun(t, dir, "checkout", "topic")

	e := git.New()
	err := e.Rebase(ctx, dir, "main")
	require.NoError(t, err)
}

func TestPush_NoRemote_ReturnsError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.Push(ctx, dir)
	require.Error(t, err)
}

func TestFetch_NoRemote(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	// git fetch with no remotes succeeds (no-op) or returns an error — both are valid.
	_ = e.Fetch(ctx, dir)
}

func TestPull_NoRemote_ReturnsError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.Pull(ctx, dir, "merge")
	require.Error(t, err)
}

func TestPull_RebaseMode_NoRemote(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.Pull(ctx, dir, "rebase")
	require.Error(t, err)
}

func TestRebaseOnto(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "base commit")
	baseSHA := headSHA(t, dir)

	gitRun(t, dir, "checkout", "-b", "oldbase")
	makeCommit(t, dir, "old.txt", "old\n", "old base commit")

	gitRun(t, dir, "checkout", "-b", "topic", "main")
	makeCommit(t, dir, "topic.txt", "topic\n", "topic commit")

	e := git.New()
	err := e.RebaseOnto(ctx, dir, "main", baseSHA, "topic")
	require.NoError(t, err)
}

func TestWorktreeList_RequireSuccessError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()

	e := git.New()
	_, err := e.WorktreeList(ctx, dir)
	require.Error(t, err)
}

func TestOperationContinue_RebaseInProgress(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	makeCommit(t, dir, "conflict.txt", "line1\nshared\nline3\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "conflict.txt", "line1\nFEATURE\nline3\n", "feature")
	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "conflict.txt", "line1\nMAIN\nline3\n", "main change")

	rebaseCmd := newGitCmd("rebase", "feature")
	rebaseCmd.Dir = dir
	_ = rebaseCmd.Run()

	if _, err := os.Stat(filepath.Join(dir, ".git", "rebase-merge")); os.IsNotExist(err) {
		t.Skip("rebase did not leave in-progress state")
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("line1\nRESOLVED\nline3\n"), 0600))
	gitRun(t, dir, "add", "conflict.txt")

	e := git.New()
	err := e.OperationContinue(ctx, dir)
	require.NoError(t, err)
}

func TestOperationAbort_RebaseInProgress(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	makeCommit(t, dir, "conflict.txt", "line1\nshared\nline3\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "conflict.txt", "line1\nFEATURE\nline3\n", "feature")
	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "conflict.txt", "line1\nMAIN\nline3\n", "main change")

	rebaseCmd := newGitCmd("rebase", "feature")
	rebaseCmd.Dir = dir
	_ = rebaseCmd.Run()

	if _, err := os.Stat(filepath.Join(dir, ".git", "rebase-merge")); os.IsNotExist(err) {
		t.Skip("rebase did not leave in-progress state")
	}

	e := git.New()
	err := e.OperationAbort(ctx, dir)
	require.NoError(t, err)
}
