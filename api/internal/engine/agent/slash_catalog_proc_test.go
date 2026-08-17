//go:build !windows

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRegression_CatalogProbeCancel_KillsDescendants pins spec §12: cancelling a
// probe must terminate the process TREE, not just the process Crowbar forked.
// exec.CommandContext's default Cancel signals one pid, so a provider CLI that
// shells out used to leave orphans running after the HTTP request was abandoned.
func TestRegression_CatalogProbeCancel_KillsDescendants(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	exec := &boundedCatalogExecutor{
		executable:   "/bin/sh",
		cwd:          dir,
		env:          os.Environ(),
		stdoutBudget: newOutputBudget(1 << 20),
		stderrBudget: newOutputBudget(1 << 20),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = exec.Run(ctx, []string{
			"-c",
			// A grandchild that outlives its parent unless the GROUP is signalled.
			"sh -c 'while :; do sleep 0.05; done' & echo $! > " + pidFile + "; sleep 60",
		})
	}()

	pid := waitForPIDFile(t, pidFile)
	require.NoError(t, syscall.Kill(pid, 0), "grandchild should be alive before cancel")

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("probe did not return after cancellation")
	}

	require.Eventually(t, func() bool {
		return syscall.Kill(pid, 0) != nil
	}, 5*time.Second, 25*time.Millisecond, "cancelled probe left an orphaned descendant")
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil || parsed <= 0 {
			return false
		}
		pid = parsed
		return true
	}, 10*time.Second, 25*time.Millisecond, "grandchild never reported its pid")
	return pid
}
