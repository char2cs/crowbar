package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestCreateBranch_AlreadyExists_ReturnsSentinel exercises reclassifyError's
// matched-sentinel branch through the real engine: `git branch -- <name>` on a
// name that already exists fails with exit 128 and "already exists" in
// stderr, which the error-rule table maps to ErrBranchAlreadyExists.
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

// TestSwitchBranch_UnknownName_ReturnsUnclassifiedError covers reclassifyError's
// final fallthrough: `git checkout <unknown>` fails at exit 1 with "did not
// match any", but every rule for that message requires exit 128, so no
// sentinel matches and the raw GitError must come back unchanged (still a
// real, non-nil error — callers still see the failure, just not as one of the
// named sentinels).
func TestSwitchBranch_UnknownName_ReturnsUnclassifiedError(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	err := e.SwitchBranch(ctx, dir, "no-such-branch")

	require.Error(t, err)
	assert.False(t, errors.Is(err, git.ErrBranchNotFound), "exit-1 checkout failures are not classified as ErrBranchNotFound today")
}

// TestReclassifyError_NonGitErrorPassesThrough is a white-box test (via
// export_test.go) for the one reclassifyError branch unreachable from any
// public engine method: branches.Create/Rename/Switch only ever return nil or
// a *gitexec.GitError, so the "err is some other error type" branch needs
// direct injection.
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

// TestStageHunk_StaleHunkID_ReturnsErrStaleHunk covers applyHunk's error
// branch: HunkPatch cannot find hunkID in the current diff (the frontend's
// diff view is stale relative to the working tree) and StageHunk must
// propagate that failure rather than silently no-op.
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
