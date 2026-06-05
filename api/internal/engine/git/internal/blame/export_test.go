// export_test.go exposes internals for white-box unit tests.
package blame

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// ExportedParsePorcelain wraps parsePorcelain for use in external test packages.
func ExportedParsePorcelain(
	output string,
) ([]domain.BlameEntry, error) {
	return parsePorcelain(output)
}

// ExportedParseUnixTime wraps parseUnixTime for use in external test packages.
func ExportedParseUnixTime(
	s string,
) time.Time {
	return parseUnixTime(s)
}

// ExportedIsCommitHeader wraps isCommitHeader for use in external test packages.
func ExportedIsCommitHeader(
	line string,
) bool {
	return isCommitHeader(line)
}

func errRunner(
	_ context.Context,
	_ string,
	_ ...string,
) exec.Result {
	return exec.Result{ExitCode: 1, Stderr: "injected runner error"}
}

// SetErrorRunner replaces gitRunner with a failing stub for the duration of a test.
func SetErrorRunner(
	cleanup func(func()),
) {
	orig := gitRunner
	gitRunner = errRunner
	cleanup(func() { gitRunner = orig })
}
