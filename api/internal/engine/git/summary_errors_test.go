package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func TestCommitsAheadOf_RevListFails(t *testing.T) {
	e := git.NewWithExec(func(_ context.Context, _ string, _ ...string) gitexec.Result {
		return gitexec.Result{ExitCode: 128, Stderr: "fatal: bad object"}
	})

	got, ok := git.ExportedCommitsAheadOf(t, e, context.Background(), "/repo", "deadbeef")

	assert.False(t, ok)
	assert.Zero(t, got)
}

func TestCommitsAheadOf_NonNumericOutput(t *testing.T) {
	e := git.NewWithExec(func(_ context.Context, _ string, _ ...string) gitexec.Result {
		return gitexec.Result{ExitCode: 0, Stdout: "not-a-number\n"}
	})

	got, ok := git.ExportedCommitsAheadOf(t, e, context.Background(), "/repo", "deadbeef")

	assert.False(t, ok)
	assert.Zero(t, got)
}

func TestNumstatFromBase_DiffFails(t *testing.T) {
	e := git.NewWithExec(func(_ context.Context, _ string, _ ...string) gitexec.Result {
		return gitexec.Result{ExitCode: 1, Stderr: "fatal: bad revision"}
	})

	added, deleted, err := git.ExportedNumstatFromBase(t, e, context.Background(), "/repo", "deadbeef")

	assert.NoError(t, err)
	assert.Zero(t, added)
	assert.Zero(t, deleted)
}

func TestResolveDiffBase_BlankMergeBaseOutput_TreatedAsNoCandidate(t *testing.T) {
	calls := 0
	e := git.NewWithExec(func(_ context.Context, _ string, args ...string) gitexec.Result {
		if len(args) > 0 && args[0] == "merge-base" {
			calls++
			return gitexec.Result{ExitCode: 0, Stdout: "   \n"}
		}
		return gitexec.Result{ExitCode: 0, Stdout: "1\n"}
	})

	got := git.ExportedResolveDiffBase(t, e, context.Background(), "/repo", "main")

	assert.Empty(t, got, "a blank merge-base result must not be adopted as the resolved base")
	assert.Equal(t, 2, calls, "both the origin/main and local main candidates must be probed")
}

func TestLooksLikeCommitSHA(t *testing.T) {
	assert.True(t, git.ExportedLooksLikeCommitSHA("0123456789abcdef0123456789abcdef01234567"), "40 lowercase hex chars must pass")
	assert.False(t, git.ExportedLooksLikeCommitSHA("main"), "a short branch name must not be mistaken for a SHA")
	assert.False(t,
		git.ExportedLooksLikeCommitSHA("0123456789abcdef0123456789abcdeg01234567"),
		"40 characters containing a non-hex digit ('g') must be rejected, not accepted on length alone",
	)
}
