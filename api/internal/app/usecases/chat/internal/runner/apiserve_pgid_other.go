//go:build !windows && !linux

package runner

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the serve process at the head of its own group, so a
// signal reaches whatever it started (a wrapper and the binary it runs).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
