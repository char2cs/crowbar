// Package runner (file apiserve.go) starts the provider's control-plane
// process and waits for it to be reachable: the `serve` fork, the socket it
// must bind, and where that socket lives.
//
// Split from apiconn.go, which is about the CONNECTION — handshake, pumping,
// teardown — once that file outgrew being readable in one sitting. This half
// is pure process and filesystem; it knows nothing about events.
package runner

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

// apiSocketPath derives a short path under the OS temp dir, keyed by a hash of
// runnerID — mirroring internal/core/gateway/transports.overrideSocketPath's own
// convention. It must be short and NEVER under a Crowbar worktree: macOS's
// sun_path is a hard 104 bytes, and a worktree-rooted tmpDir routinely exceeds
// it (see [[project_dev_home_isolation]]).
func apiSocketPath(runnerID string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(runnerID))
	return filepath.Join(os.TempDir(), fmt.Sprintf("crowbar-api-%x.sock", h.Sum64()))
}

// forkServeProcess starts argv as a long-lived BACKGROUND process — not a PTY:
// codex's app-server is a headless control-plane process, and the PTY the rest
// of spawnRunner manages is reserved for `attach`, if the descriptor declares
// one.
func forkServeProcess(argv []string) (*serveProcess, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("agent: api transport: empty serve argv")
	}
	cmd := exec.Command(binpath.Resolve(argv[0]), argv[1:]...) //nolint:gosec // argv is descriptor-declared and template-expanded, not user input
	cmd.Stdout = nil
	cmd.Stderr = nil
	ownProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("agent: api transport: start %s: %w", argv[0], err)
	}
	return reapServe(cmd), nil
}

// serveExitBound is how long a teardown waits for a killed `serve` to be gone.
const serveExitBound = 5 * time.Second

// serveProcess is a started `serve` and the one goroutine that reaps it:
// exited closes when it is gone, which is what a teardown and the runner's
// exit watch both wait on.
type serveProcess struct {
	cmd    *exec.Cmd
	exited chan struct{}
}

func reapServe(cmd *exec.Cmd) *serveProcess {
	s := &serveProcess{cmd: cmd, exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()
	return s
}

// kill ends the process and everything it started, and waits, bounded, until
// it is gone. Waiting is the point: codex holds a writer lock on its thread for
// as long as it lives, and a replacement resuming that thread before the old
// process is reaped is refused ("already has an active writer").
func (s *serveProcess) kill() {
	if s == nil || s.cmd.Process == nil {
		return
	}
	// Asked first: codex releases its thread's writer lease only on a graceful
	// exit — after a SIGKILL the next app-server is refused the thread.
	if signalGroup(s.cmd.Process, syscall.SIGTERM) == nil {
		select {
		case <-s.exited:
		case <-time.After(serveExitBound):
		}
	}
	// Whatever is still in the group — the leader past its bound, or a child
	// it left behind — is not allowed to outlive the runner.
	_ = signalGroup(s.cmd.Process, syscall.SIGKILL)
	select {
	case <-s.exited:
	case <-time.After(serveExitBound):
	}
}

// waitForSocket polls for sockPath to exist, bounded by ctx. codex's app-server
// creates the socket file synchronously on bind, so a short poll is enough —
// there is no readiness protocol beyond the file's existence.
func waitForSocket(ctx context.Context, sockPath string, exited <-chan struct{}) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(sockPath); err == nil {
			return nil
		}
		select {
		case <-ticker.C:
			continue
		case <-deadline.C:
			return fmt.Errorf("agent: api transport: socket %s never appeared", sockPath)
		case <-exited:
			return fmt.Errorf("agent: api transport: serve exited before opening %s", sockPath)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
