// export_test.go exposes internals for white-box unit tests.
package stash

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// ExportedParseLine wraps parseLine for use in external test packages.
func ExportedParseLine(
	line string,
) (interface{}, bool) {
	s, ok := parseLine(line)
	return s, ok
}

func errRunner(
	_ context.Context,
	_ string,
	_ ...string,
) (exec.Result, error) {
	return exec.Result{}, errors.New("injected runner error")
}

// SetErrorRunner replaces gitRunner with a failing stub for the duration of a test.
func SetErrorRunner(
	cleanup func(func()),
) {
	orig := gitRunner
	gitRunner = errRunner
	cleanup(func() { gitRunner = orig })
}
