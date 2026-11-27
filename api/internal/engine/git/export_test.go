package git

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func SameRepoMutex(
	ctx context.Context,
	a string,
	b string,
) bool {
	e := New().(*engine)
	return e.repoMutex(ctx, a) == e.repoMutex(ctx, b)
}

func ExportedResolveCommonDir(
	ctx context.Context,
	repoPath string,
) string {
	e := New().(*engine)
	return e.resolveCommonDir(ctx, repoPath)
}

func ExportedComputeStatus(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	e := New().(*engine)
	return e.ComputeStatus(ctx, repoPath)
}

func ExportedComputeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	e := New().(*engine)
	return e.computeWorkingTreeSummary(ctx, repoPath, forkPointSha)
}

func ExportedParseWorktreeList(
	output string,
) []WorktreeEntry {
	return parseWorktreeList(output)
}

func ExportedParseNumstat(
	output string,
) (int, int, error) {
	return parseNumstat(output)
}

func ExportedCommitsAheadOf(
	tb testing.TB,
	e Engine,
	ctx context.Context,
	repoPath string,
	ref string,
) (int, bool) {
	tb.Helper()
	return engineOf(tb, e).commitsAheadOf(ctx, repoPath, ref)
}

func ExportedNumstatFromBase(
	tb testing.TB,
	e Engine,
	ctx context.Context,
	repoPath string,
	base string,
) (int, int, error) {
	tb.Helper()
	return engineOf(tb, e).numstatFromBase(ctx, repoPath, base)
}

func ExportedResolveDiffBase(
	tb testing.TB,
	e Engine,
	ctx context.Context,
	repoPath string,
	base string,
) string {
	tb.Helper()
	got, _ := engineOf(tb, e).resolveDiffBase(ctx, repoPath, base)
	return got
}

func ExportedLooksLikeCommitSHA(
	s string,
) bool {
	return looksLikeCommitSHA(s)
}

func ExportedIsStaleWorktreeConflict(
	err error,
) bool {
	return isStaleWorktreeConflict(err)
}

func ExportedReclassifyError(
	err error,
) error {
	return reclassifyError(err)
}

func ExportedDetectInProgressOp(
	ctx context.Context,
	repoPath string,
) string {
	return detectInProgressOp(ctx, repoPath)
}

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

func NewWithErrorExec() Engine {
	return &engine{
		exec:      errExec,
		execStdin: errExecStdin,
	}
}

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

func engineOf(
	tb testing.TB,
	e Engine,
) *engine {
	tb.Helper()
	eng, ok := e.(*engine)
	require.True(tb, ok, "expected *engine, got %T", e)
	return eng
}
