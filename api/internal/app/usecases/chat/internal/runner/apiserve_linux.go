package runner

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup puts the serve process at the head of its own group, so a
// signal reaches whatever it started (a wrapper and the binary it runs), and
// has it asked to stop if the daemon dies without its shutdown.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
