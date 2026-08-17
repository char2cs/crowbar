//go:build !windows

package exec

import (
	"os"
	osexec "os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// isolateProcessGroup makes the command its own process-group leader (pgid ==
// pid) so cancellation can signal the whole group. Without it a provider CLI
// that forks its own helpers leaves those helpers running after the command is
// killed: exec.CommandContext only ever signals the direct child.
func isolateProcessGroup(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree signals the group (negative pid), falling back to the single
// process when the group send fails — e.g. ESRCH because the child changed its
// own pgid — so a stale group id never masks a live, killable process.
func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	if err := unix.Kill(-proc.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return proc.Kill()
}
