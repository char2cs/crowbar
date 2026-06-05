// export_test.go exposes internal functions for white-box unit tests.
package branches

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// ExportedParseRecord wraps parseRecord for use in _test packages.
func ExportedParseRecord(rec string) (domain.Branch, bool) {
	return parseRecord(rec)
}

// ExportedParseList wraps parseList for use in _test packages.
func ExportedParseList(output string) []domain.Branch {
	return parseList(output)
}

// ExportedParseTrack wraps parseTrack for use in _test packages.
func ExportedParseTrack(track string, b *domain.Branch) {
	parseTrack(track, b)
}

// ExportedParseTrackSegment wraps parseTrackSegment for use in _test packages.
func ExportedParseTrackSegment(seg string, b *domain.Branch) {
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
