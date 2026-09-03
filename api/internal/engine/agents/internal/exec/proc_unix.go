//go:build !windows

package exec

import (
	"os"
	osexec "os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func isolateProcessGroup(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	if err := unix.Kill(-proc.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return proc.Kill()
}
