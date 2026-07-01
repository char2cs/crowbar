// export_test.go exposes internal functions for white-box unit tests.
package branches

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// ExportedParseRecord wraps parseRecord for use in _test packages.
func ExportedParseRecord(rec string) (gitdomain.Branch, bool) {
	return parseRecord(rec)
}

// ExportedParseList wraps parseList for use in _test packages.
func ExportedParseList(output string) []gitdomain.Branch {
	return parseList(output)
}

// ExportedParseTrack wraps parseTrack for use in _test packages.
func ExportedParseTrack(track string, b *gitdomain.Branch) {
	parseTrack(track, b)
}

// ExportedParseTrackSegment wraps parseTrackSegment for use in _test packages.
func ExportedParseTrackSegment(seg string, b *gitdomain.Branch) {
	parseTrackSegment(seg, b)
}

// errRunner is a gitRunner replacement that always returns a non-nil error.
// Used to exercise the err != nil branches that exec.Git itself never triggers.
func errRunner(
	_ context.Context,
	_ string,
	_ ...string,
) exec.Result {
	return exec.Result{ExitCode: 1, Stderr: "injected runner error"}
}

// SetErrorRunner replaces gitRunner with a failing stub for the duration of a
// test, then restores the original on cleanup.  Call from export_test.go tests.
func SetErrorRunner(cleanup func(func())) {
	orig := gitRunner
	gitRunner = errRunner
	cleanup(func() { gitRunner = orig })
}

// SetRecordingRunner replaces gitRunner with a stub that records every
// invocation's args and returns benign success, restoring the original on
// cleanup. The returned pointer accumulates the recorded arg lists so tests can
// assert a `--` end-of-options separator precedes a user operand.
func SetRecordingRunner(cleanup func(func())) *[][]string {
	var recorded [][]string
	orig := gitRunner
	gitRunner = func(_ context.Context, _ string, args ...string) exec.Result {
		captured := make([]string, len(args))
		copy(captured, args)
		recorded = append(recorded, captured)
		return exec.Result{ExitCode: 0}
	}
	cleanup(func() { gitRunner = orig })
	return &recorded
}
