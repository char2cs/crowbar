//go:build !windows

package session

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func terminateSignal(proc *os.Process) error {
	if err := unix.Kill(-proc.Pid, syscall.SIGTERM); err == nil {
		return nil
	}
	return proc.Signal(syscall.SIGTERM)
}

func killSignal(proc *os.Process) error {
	if err := unix.Kill(-proc.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("session: kill pid %d: %w", proc.Pid, err)
	}
	return nil
}
