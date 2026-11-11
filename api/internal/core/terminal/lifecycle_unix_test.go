//go:build unix

package terminal_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// TestKill_SignalsProcessGroup: Kill must take the session's children down with it, as
// Terminate does — a background child of a command session must not survive it.
func TestKill_SignalsProcessGroup(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	id, err := eng.CreateCommand(ctx, "chat-pg", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 300 & echo $! > " + pidFile + "; wait"}, nil, nil)
	require.NoError(t, err)

	var pid int
	require.Eventually(t, func() bool {
		b, err := os.ReadFile(pidFile)
		if err != nil || len(b) == 0 {
			return false
		}
		_, scanErr := fmtSscan(string(b), &pid)
		return scanErr == nil && pid > 0
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, eng.Kill(ctx, id))
	require.Eventually(t, func() bool { return !processAlive(pid) }, 5*time.Second, 10*time.Millisecond,
		"the session's background child must die with it")
}

func fmtSscan(s string, pid *int) (int, error) { return fmt.Sscan(strings.TrimSpace(s), pid) }

// processAlive reports whether pid is a running (non-zombie) process. A container's init
// may never reap an orphan, so a dead child can linger as a zombie; that counts as dead.
func processAlive(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return syscall.Kill(pid, 0) == nil // no procfs (darwin): best effort
	}
	fields := strings.Fields(string(b))
	return len(fields) > 2 && fields[2] != "Z"
}
