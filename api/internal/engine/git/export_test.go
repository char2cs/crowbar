// export_test.go exposes unexported types/functions for white-box tests.
package git

import (
	"context"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// SameRepoMutex reports whether two repo paths resolve to the same per-repo
// mutex (i.e. share a git common directory) for the engine returned by New().
func SameRepoMutex(
	ctx context.Context,
	a string,
	b string,
) bool {
	e := New().(*engine)
	return e.repoMutex(ctx, a) == e.repoMutex(ctx, b)
}

// ExportedResolveCommonDir returns the lock key the engine derives for repoPath.
func ExportedResolveCommonDir(
	ctx context.Context,
	repoPath string,
) string {
	e := New().(*engine)
	return e.resolveCommonDir(ctx, repoPath)
}

// ExportedComputeStatus calls ComputeStatus on the engine returned by New().
func ExportedComputeStatus(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	e := New().(*engine)
	return e.ComputeStatus(ctx, repoPath)
}

// ExportedComputeWorkingTreeSummary calls ComputeWorkingTreeSummary.
func ExportedComputeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	e := New().(*engine)
	return e.ComputeWorkingTreeSummary(ctx, repoPath, forkPointSha)
}

// ExportedParseWorktreeList wraps parseWorktreeList for unit tests.
func ExportedParseWorktreeList(
	output string,
) []WorktreeEntry {
	return parseWorktreeList(output)
}

// ExportedParseNumstat wraps parseNumstat for unit tests.
func ExportedParseNumstat(
	output string,
) (int, int, error) {
	return parseNumstat(output)
}

// ExportedDetectInProgressOp wraps detectInProgressOp for unit tests.
func ExportedDetectInProgressOp(
	ctx context.Context,
	repoPath string,
) string {
	return detectInProgressOp(ctx, repoPath)
}

// SetNetTimeoutsForTest overrides the network git timeouts and returns a
// restore func, so tests can exercise the stalled-remote bound without
// waiting minutes.
func SetNetTimeoutsForTest(
	transfer time.Duration,
	query time.Duration,
) func() {
	origTransfer, origQuery := netTransferTimeout, netQueryTimeout
	netTransferTimeout, netQueryTimeout = transfer, query
	return func() {
		netTransferTimeout, netQueryTimeout = origTransfer, origQuery
	}
}

// NewWithExec builds an engine over a fake exec function, for white-box
// concurrency tests that need to control command timing without a real git
// subprocess.
func NewWithExec(
	exec func(ctx context.Context, dir string, args ...string) gitexec.Result,
) Engine {
	return &engine{exec: exec}
}

func errExec(_ context.Context, _ string, _ ...string) gitexec.Result {
	return gitexec.Result{ExitCode: 1, Stderr: "injected exec error"}
}

func errExecStdin(_ context.Context, _, _ string, _ ...string) gitexec.Result {
	return gitexec.Result{ExitCode: 1, Stderr: "injected exec stdin error"}
}

// NewWithErrorExec returns an engine whose exec always returns an error.
func NewWithErrorExec() Engine {
	return &engine{
		exec:      errExec,
		execStdin: errExecStdin,
	}
}

// NewWithRecordingExec returns an Engine whose every git invocation is recorded
// (the exec returns a benign success) and a pointer to the slice of recorded
// argument lists. It lets tests assert the exact args (e.g. that a `--`
// end-of-options separator precedes a user operand) without spawning git.
func NewWithRecordingExec() (Engine, *[][]string) {
	var recorded [][]string
	rec := func(_ context.Context, _ string, args ...string) gitexec.Result {
		captured := make([]string, len(args))
		copy(captured, args)
		recorded = append(recorded, captured)
		return gitexec.Result{ExitCode: 0}
	}
	e := &engine{
		exec: rec,
		execStdin: func(_ context.Context, _, _ string, args ...string) gitexec.Result {
			captured := make([]string, len(args))
			copy(captured, args)
			recorded = append(recorded, captured)
			return gitexec.Result{ExitCode: 0}
		},
	}
	return e, &recorded
}
