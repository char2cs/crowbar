package server

import (
	"fmt"
	"io"
	"os/exec"
)

// processTransport adapts a child process's stdin/stdout pipes to an
// io.ReadWriteCloser: reads come from stdout, writes go to stdin, and Close
// shuts the pipes and kills the process.
type processTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func newProcessTransport(
	cmd *exec.Cmd,
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
	return &processTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
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

func (t *processTransport) Close() error {
	_ = t.stdin.Close()
	_ = t.stdout.Close()
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return nil
}
