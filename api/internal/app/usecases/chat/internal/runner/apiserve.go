// Package runner (file apiserve.go) starts the provider's control-plane
// process: the `serve` fork, whose stdin and stdout carry its api connection.
//
// Split from apiconn.go, which is about the CONNECTION — handshake, pumping,
// teardown. This half is pure process; it knows nothing about events.
package runner

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// forkServeProcess starts argv as a long-lived BACKGROUND process — not a PTY,
// which is reserved for `attach`, if the descriptor declares one. Its stdin
// and stdout are the api connection's stdio; its stderr is a log nothing reads.
func forkServeProcess(argv []string) (*serveProcess, engineagents.APIStdio, error) {
	if len(argv) == 0 {
		return nil, engineagents.APIStdio{}, fmt.Errorf("agent: api transport: empty serve argv")
	}
	cmd := exec.Command(binpath.Resolve(argv[0]), argv[1:]...) //nolint:gosec // argv is descriptor-declared and template-expanded, not user input
	ownProcessGroup(cmd)
	stdio, err := engineagents.StartServe(cmd)
	if err != nil {
		return nil, engineagents.APIStdio{}, fmt.Errorf("agent: api transport: start %s: %w", argv[0], err)
	}
	return reapServe(cmd), stdio, nil
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
// it is gone. Waiting is the point: a provider can hold a writer lock on its
// session for as long as serve lives, and refuse a replacement resuming it
// before the old process is reaped.
func (s *serveProcess) kill() {
	if s == nil || s.cmd.Process == nil {
		return
	}
	// Asked first: a provider may release its session's lease only on a
	// graceful exit, so after a SIGKILL the next serve is refused the session.
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
