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

func unexpectedExit(waitErr error, deliberate bool) error {
	var exitErr *exec.ExitError
	if deliberate && errors.As(waitErr, &exitErr) {
		return nil
	}
	return waitErr
}

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

func gone(op string, err error) error {
	if err == nil || errors.Is(err, os.ErrClosed) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return fmt.Errorf("process: %s: %w", op, err)
}

func closeLogged(what string, c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Warn("lsp: "+what, "err", err)
	}
}
