//go:build !windows

package runner

import (
	"os"
	"syscall"
)

// signalGroup signals every process in proc's group; the group outlives its
// leader, so this also reaches a child still running after the leader exited.
// A process that heads no group of its own is signalled alone.
func signalGroup(proc *os.Process, sig syscall.Signal) error {
	if err := syscall.Kill(-proc.Pid, sig); err == nil {
		return nil
	}
	return proc.Signal(sig)
}
