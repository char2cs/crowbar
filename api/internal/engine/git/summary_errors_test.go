package git_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// TestCommitDistanceToHead_RevListFails covers the branch a real repo cannot
// reach: commitDistanceToHead is only ever called on a rev that `git
// merge-base` just resolved successfully, so `rev-list --count` failing on it
// needs a fake exec (a corrupted object database, an interrupted subprocess).
func TestCommitDistanceToHead_RevListFails(t *testing.T) {
	e := git.NewWithExec(func(_ context.Context, _ string, _ ...string) gitexec.Result {
		return gitexec.Result{ExitCode: 128, Stderr: "fatal: bad object"}
	})

	got := git.ExportedCommitDistanceToHead(e, context.Background(), "/repo", "deadbeef")

	assert.Equal(t, math.MaxInt, got)
}

// TestCommitDistanceToHead_NonNumericOutput covers the strconv.Atoi failure
// path: rev-list exits 0 but its stdout is not a bare integer.
func TestCommitDistanceToHead_NonNumericOutput(t *testing.T) {
	e := git.NewWithExec(func(_ context.Context, _ string, _ ...string) gitexec.Result {
		return gitexec.Result{ExitCode: 0, Stdout: "not-a-number\n"}
	})

	got := git.ExportedCommitDistanceToHead(e, context.Background(), "/repo", "deadbeef")

	assert.Equal(t, math.MaxInt, got)
}

// TestNumstatFromBase_DiffFails covers numstat's ExitCode != 0 branch, which
// a real resolved base practically never triggers.
func TestNumstatFromBase_DiffFails(t *testing.T) {
	e := git.NewWithExec(func(_ context.Context, _ string, _ ...string) gitexec.Result {
		return gitexec.Result{ExitCode: 1, Stderr: "fatal: bad revision"}
	})

	added, deleted, err := git.ExportedNumstatFromBase(e, context.Background(), "/repo", "deadbeef")

	assert.NoError(t, err)
	assert.Zero(t, added)
	assert.Zero(t, deleted)
}

// TestResolveDiffBase_BlankMergeBaseOutput_TreatedAsNoCandidate covers
// resolveDiffBase's defensive guard against a merge-base invocation that exits
// 0 but prints nothing: real git never does this (a successful merge-base
// always writes a SHA), so the only way to drive it is a fake exec. Both
// candidates ("origin/main" and "main") are made to return the same blank
// result, so the branch is hit on every iteration of the loop, not just the
// first.
func TestResolveDiffBase_BlankMergeBaseOutput_TreatedAsNoCandidate(t *testing.T) {
	calls := 0
	e := git.NewWithExec(func(_ context.Context, _ string, args ...string) gitexec.Result {
		if len(args) > 0 && args[0] == "merge-base" {
			calls++
			return gitexec.Result{ExitCode: 0, Stdout: "   \n"}
		}
		return gitexec.Result{ExitCode: 0}
	})

	got := git.ExportedResolveDiffBase(e, context.Background(), "/repo", "main")

	assert.Empty(t, got, "a blank merge-base result must not be adopted as the resolved base")
	assert.Equal(t, 2, calls, "both the origin/main and local main candidates must be probed")
}

// TestLooksLikeCommitSHA covers the three ways a 40-character-or-not string can
// fail to be treated as a resolved commit SHA: wrong length, and the right
// length but containing a non-hex character (which the loop must catch rather
// than accepting on length alone).
func TestLooksLikeCommitSHA(t *testing.T) {
	assert.True(t, git.ExportedLooksLikeCommitSHA("0123456789abcdef0123456789abcdef01234567"), "40 lowercase hex chars must pass")
	assert.False(t, git.ExportedLooksLikeCommitSHA("main"), "a short branch name must not be mistaken for a SHA")
	assert.False(t,
		git.ExportedLooksLikeCommitSHA("0123456789abcdef0123456789abcdeg01234567"),
		"40 characters containing a non-hex digit ('g') must be rejected, not accepted on length alone",
	)
}
