//go:build windows

package agent

import (
	"os"
	"os/exec"
)

// isolateProcessGroup is a no-op on Windows: descendant containment there needs
// a job object, which this package does not create.
func isolateProcessGroup(*exec.Cmd) {}

func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	return proc.Kill()
}
