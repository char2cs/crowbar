package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// processTransport adapts a child process's stdin/stdout pipes to an
// io.ReadWriteCloser: reads come from stdout, writes go to stdin, and Close
// shuts the pipes and kills the process.
//
// A single reaper goroutine owns the one and only cmd.Wait(): it blocks until
// the process exits — whether on its own (crash/OOM) or because Close killed it
// — reaps the zombie, and signals completion. On a NATURAL exit (Close did not
// initiate the teardown) it also fires onExit so the manager can evict the dead
// pool entry, which is what stops a corpse from being handed back to the next
// request (R10). Close does not call cmd.Wait() itself — that would double-Wait
// the reaper — it kills the process and waits for the reaper to finish.
type processTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	onExit func()

	closeOnce sync.Once
	reaped    chan struct{}
	mu        sync.Mutex
	closing   bool
}

func newProcessTransport(
	cmd *exec.Cmd,
	onExit func(),
) (io.ReadWriteCloser, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("process: start: %w", err)
	}
	t := &processTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		onExit: onExit,
		reaped: make(chan struct{}),
	}
	safego.Go("lsp.server.reap", t.reap)
	return t, nil
}

func (t *processTransport) Read(
	p []byte,
) (int, error) {
	return t.stdout.Read(p)
}

func (t *processTransport) Write(
	p []byte,
) (int, error) {
	return t.stdin.Write(p)
}

// reap is the single cmd.Wait() owner. It blocks until the process exits, reaps
// it, and signals via the reaped channel. When the exit was NOT initiated by
// Close (a genuine self-exit), it fires onExit so the dead server is evicted.
func (t *processTransport) reap() {
	waitErr := t.cmd.Wait()
	close(t.reaped)

	t.mu.Lock()
	deliberate := t.closing
	t.mu.Unlock()
	if err := unexpectedExit(waitErr, deliberate); err != nil {
		slog.Warn("lsp: language server exited", "cmd", t.cmd.Path, "err", err)
	}
	if !deliberate && t.onExit != nil {
		t.onExit()
	}
}

// unexpectedExit is what a Wait failure means: a server Close killed exits
// non-zero by design, anything else (a crash, a failed Wait) is worth a line.
func unexpectedExit(waitErr error, deliberate bool) error {
	var exitErr *exec.ExitError
	if deliberate && errors.As(waitErr, &exitErr) {
		return nil
	}
	return waitErr
}

// Close kills the process and the pipes, then blocks until the reaper has
// finished cmd.Wait() so cmd.ProcessState is set on return. It is idempotent and
// suppresses the onExit callback (the teardown is deliberate, not a crash). Only
// the first call reports a failure to close or kill.
func (t *processTransport) Close() error {
	var err error
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closing = true
		t.mu.Unlock()

		err = errors.Join(gone("close stdin", t.stdin.Close()), gone("close stdout", t.stdout.Close()))
		if t.cmd.Process != nil {
			err = errors.Join(err, gone("kill", t.cmd.Process.Kill()))
		}
	})
	<-t.reaped
	return err
}

// gone wraps a teardown failure, except that closing what the reaper already
// closed or killing what already exited is the teardown having been done.
func gone(op string, err error) error {
	if err == nil || errors.Is(err, os.ErrClosed) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return fmt.Errorf("process: %s: %w", op, err)
}

// closeLogged closes a transport nobody can return the failure to.
func closeLogged(what string, c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Warn("lsp: "+what, "err", err)
	}
}
