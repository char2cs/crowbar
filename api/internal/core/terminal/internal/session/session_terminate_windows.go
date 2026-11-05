//go:build windows

package session

import (
	"errors"
	"fmt"
	"os"
)

// terminateSignal is an immediate hard kill on Windows, which has no POSIX signals
// (os.Process.Signal supports only os.Kill); there is no graceful flush.
func terminateSignal(proc *os.Process) error {
	return proc.Kill()
}

// killSignal hard-kills the process; Windows has no process-group signal to send. A child
// that already exited is not an error.
func killSignal(proc *os.Process) error {
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("session: kill pid %d: %w", proc.Pid, err)
	}
	return nil
}
