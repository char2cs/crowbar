// export_test.go exposes unexported types/functions for white-box tests.
package git

import (
	"context"
	"errors"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ExportedComputeStatus calls ComputeStatus on the engine returned by New().
func ExportedComputeStatus(
	ctx context.Context,
	repoPath string,
) (domain.GitStatus, error) {
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
	repoPath string,
) string {
	return detectInProgressOp(repoPath)
}

func errExec(_ context.Context, _ string, _ ...string) (gitexec.Result, error) {
	return gitexec.Result{}, errors.New("injected exec error")
}

func errExecStdin(_ context.Context, _ string, _ string, _ ...string) (gitexec.Result, error) {
	return gitexec.Result{}, errors.New("injected exec stdin error")
}

// NewWithErrorExec returns an engine whose exec always returns an error.
func NewWithErrorExec() Engine {
	return &engine{
		exec:      errExec,
		execStdin: errExecStdin,
	}
}
