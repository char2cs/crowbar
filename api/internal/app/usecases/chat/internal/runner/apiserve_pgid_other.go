//go:build !windows && !linux

package runner

import (
	"os/exec"
	"syscall"
)

func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
