package usecases

import (
	"os/exec"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
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
		// #nosec G204 -- the binary is the constant "git"; only sub-command
		// arguments vary, all internally constructed by defaultbranch.Resolve.
		out, err := exec.Command("git", full...).Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	}
}
