//go:build !windows

package session

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// terminateSignal sends a clean-exit SIGTERM — a PID-level action, never a PTY
// write (spec §8) — so a well-behaved child gets the chance to flush state
// before it dies, unlike Kill's unconditional SIGKILL.
//
// pty.Start (creack/pty) sets SysProcAttr.Setsid on every child spawned by
// this package (spawn/spawnCmd), so the child is always its own session AND
// process-group leader: pgid == pid. Signalling the group (a negative pid)
// reaches any grandchildren the CLI itself spawned, not just the immediate
// PID — mirroring how a real terminal hangup propagates. Falls back to a
// direct single-process signal if the group send fails (e.g. ESRCH because
// the child already changed its own pgid), so a stale/mismatched group id
// never masks a live, signalable process.
func terminateSignal(proc *os.Process) error {
	if err := unix.Kill(-proc.Pid, syscall.SIGTERM); err == nil {
		return nil
	}
	return proc.Signal(syscall.SIGTERM)
}
