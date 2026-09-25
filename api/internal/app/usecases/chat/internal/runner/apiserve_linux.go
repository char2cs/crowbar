package runner

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the serve process at the head of its own group, so a
// signal reaches whatever it started (codex's node wrapper and its binary),
// and has it asked to stop if the daemon dies without its shutdown.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
