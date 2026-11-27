package testutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
)

type LinePeer struct {
	stdin  io.ReadCloser
	lines  *bufio.Reader
	mu     sync.Mutex
	stdout io.WriteCloser
}

func NewLinePeer(stdin io.ReadCloser, stdout io.WriteCloser) *LinePeer {
	return &LinePeer{stdin: stdin, lines: bufio.NewReader(stdin), stdout: stdout}
}

func StdioPipe(tb testing.TB) (in, out *os.File, peer *LinePeer) {
	tb.Helper()
	childIn, in, err := os.Pipe()
	if err != nil {
		tb.Fatalf("stdio pipe: stdin: %v", err)
	}
	out, childOut, err := os.Pipe()
	if err != nil {
		tb.Fatalf("stdio pipe: stdout: %v", err)
	}
	tb.Cleanup(func() {
		for _, f := range []*os.File{in, out, childIn, childOut} {
			if err := f.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				tb.Errorf("stdio pipe: close: %v", err)
			}
		}
	})
	return in, out, NewLinePeer(childIn, childOut)
}

func (p *LinePeer) Read() ([]byte, error) {
	line, err := p.lines.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return nil, fmt.Errorf("line peer: read: %w", err)
	}
	return bytes.TrimSuffix(line, []byte("\n")), nil
}

func (p *LinePeer) ReadJSON(v any) error {
	line, err := p.Read()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(line, v); err != nil {
		return fmt.Errorf("line peer: unmarshal: %w", err)
	}
	return nil
}

func (p *LinePeer) Write(msg []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.stdout.Write(append(bytes.Clone(msg), '\n')); err != nil {
		return fmt.Errorf("line peer: write: %w", err)
	}
	return nil
}

func (p *LinePeer) WriteJSON(v any) error {
	msg, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("line peer: marshal: %w", err)
	}
	return p.Write(msg)
}

func (p *LinePeer) Close() error {
	return errors.Join(p.stdout.Close(), p.stdin.Close())
}
