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

// LinePeer is a fake child's end of its stdio: it reads what the parent
// writes to the child's stdin and writes the child's stdout, one JSON
// message per line.
type LinePeer struct {
	stdin  io.ReadCloser
	lines  *bufio.Reader
	mu     sync.Mutex
	stdout io.WriteCloser
}

// NewLinePeer speaks on a child's own stdin and stdout.
func NewLinePeer(stdin io.ReadCloser, stdout io.WriteCloser) *LinePeer {
	return &LinePeer{stdin: stdin, lines: bufio.NewReader(stdin), stdout: stdout}
}

// StdioPipe stands a LinePeer in for a child process: in and out are the
// parent's ends of that child's stdin and stdout.
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

// Read returns the next line the parent wrote, without its newline.
func (p *LinePeer) Read() ([]byte, error) {
	line, err := p.lines.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return nil, fmt.Errorf("line peer: read: %w", err)
	}
	return bytes.TrimSuffix(line, []byte("\n")), nil
}

// ReadJSON unmarshals the next line the parent wrote into v.
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

// Write sends msg as one line.
func (p *LinePeer) Write(msg []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.stdout.Write(append(bytes.Clone(msg), '\n')); err != nil {
		return fmt.Errorf("line peer: write: %w", err)
	}
	return nil
}

// WriteJSON sends v, marshalled, as one line.
func (p *LinePeer) WriteJSON(v any) error {
	msg, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("line peer: marshal: %w", err)
	}
	return p.Write(msg)
}

// Close hangs up both pipes, as a child that exits does.
func (p *LinePeer) Close() error {
	return errors.Join(p.stdout.Close(), p.stdin.Close())
}
