//go:build windows

package exec

import (
	"os"
	osexec "os/exec"
)

func isolateProcessGroup(*osexec.Cmd) {}

func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	return proc.Kill()
}
