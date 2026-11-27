package runner

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

const serveExitBound = 5 * time.Second

type serveProcess struct {
	cmd    *exec.Cmd
	exited chan struct{}
}

func reapServe(cmd *exec.Cmd) *serveProcess {
	s := &serveProcess{cmd: cmd, exited: make(chan struct{})}
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("agent: api transport: serve exited", "err", err)
		}
		close(s.exited)
	}()
	return s
}

func (s *serveProcess) kill() {
	if s == nil || s.cmd.Process == nil {
		return
	}
	if signalGroup(s.cmd.Process, syscall.SIGTERM) == nil {
		select {
		case <-s.exited:
		case <-time.After(serveExitBound):
		}
	}
	err := signalGroup(s.cmd.Process, syscall.SIGKILL)
	if err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		slog.Warn("agent: api transport: kill serve", "pid", s.cmd.Process.Pid, "err", err)
	}
	select {
	case <-s.exited:
	case <-time.After(serveExitBound):
	}
}
