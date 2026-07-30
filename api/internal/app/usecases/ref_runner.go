package usecases

import (
	"os/exec"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

func nowFunc() time.Time {
	return time.Now()
}

func newRefRunner(
	repoPath string,
) defaultbranch.RefRunner {
	return func(
		args ...string,
	) (string, bool) {
		full := append([]string{"-C", repoPath}, args...)
		// #nosec G204 -- the binary is git, resolved by binpath; only sub-command
		// arguments vary, all internally constructed by defaultbranch.Resolve.
		out, err := exec.Command(binpath.Git(), full...).Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	}
}
