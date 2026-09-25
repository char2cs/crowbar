//go:build !windows && !linux

package runner

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the serve process at the head of its own group, so a
// signal reaches whatever it started (codex's node wrapper and its binary).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
