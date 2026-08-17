//go:build windows

package exec

import (
	"os"
	osexec "os/exec"
)

// isolateProcessGroup is a no-op on Windows: descendant containment there needs
// a job object, which this package does not create.
func isolateProcessGroup(*osexec.Cmd) {}

func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	return proc.Kill()
}
