package branches

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func ExportedParseRecord(rec string) (gitdomain.Branch, bool) {
	return parseRecord(rec)
}

func ExportedParseList(output string) []gitdomain.Branch {
	return parseList(output)
}

func errRunner(
	_ context.Context,
	_ string,
	_ ...string,
) exec.Result {
	return exec.Result{ExitCode: 1, Stderr: "injected runner error"}
}

func SetErrorRunner(cleanup func(func())) {
	orig := gitRunner
	gitRunner = errRunner
	cleanup(func() { gitRunner = orig })
}

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
