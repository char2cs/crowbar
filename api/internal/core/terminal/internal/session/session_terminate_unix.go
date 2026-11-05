//go:build !windows

package session

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// terminateSignal SIGTERMs the child's process group (pty.Start sets Setsid, so
// pgid == pid) to give it and its descendants a chance to flush, falling back to the
// single process when the group send fails (the child changed its pgid).
func terminateSignal(proc *os.Process) error {
	if err := unix.Kill(-proc.Pid, syscall.SIGTERM); err == nil {
		return nil
	}
	return proc.Signal(syscall.SIGTERM)
}

// killSignal SIGKILLs the child's whole process group so nothing it spawned survives,
// falling back to the single process like terminateSignal. An exited child is not an error.
func killSignal(proc *os.Process) error {
	if err := unix.Kill(-proc.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("session: kill pid %d: %w", proc.Pid, err)
	}
	return nil
}
