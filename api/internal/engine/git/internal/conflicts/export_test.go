// export_test.go exposes internals for white-box unit tests.
package conflicts

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

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
