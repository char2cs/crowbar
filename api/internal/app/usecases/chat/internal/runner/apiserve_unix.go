//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the serve process at the head of its own group, so a
// signal reaches whatever it started (codex's node wrapper and its binary).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup signals every process in proc's group; the group outlives its
// leader, so this also reaches a child still running after the leader exited.
// A process that heads no group of its own is signalled alone.
func signalGroup(proc *os.Process, sig syscall.Signal) error {
	if err := syscall.Kill(-proc.Pid, sig); err == nil {
		return nil
	}
	return proc.Signal(sig)
}
