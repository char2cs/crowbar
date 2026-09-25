//go:build windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

func ownProcessGroup(*exec.Cmd) {}

func signalGroup(proc *os.Process, sig syscall.Signal) error {
	if sig == syscall.SIGKILL {
		return proc.Kill()
	}
	return proc.Signal(sig)
}
