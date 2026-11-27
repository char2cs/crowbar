package blame

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func ExportedParseUnixTime(
	s string,
) time.Time {
	return parseUnixTime(s)
}

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
	return exec.Result{ExitCode: 128, Stderr: "injected runner error"}
}

func SetErrorRunner(
	cleanup func(func()),
) {
	orig := gitRunner
	gitRunner = errRunner
	cleanup(func() { gitRunner = orig })
}
