package jsonrpc

import (
	"bufio"
	"bytes"
	"fmt"
	"time"
)

// Transport moves whole messages; framing and write deadlines are its own business.
// Recv returns an error (io.EOF at end of stream) once nothing more will arrive.
type Transport interface {
	Send([]byte) error
	Recv() ([]byte, error)
	Close() error
}

// stdioTransport frames one message per line over a child's pipes.
type stdioTransport struct {
	Stdio
	lines *bufio.Reader
}

func (s *stdioTransport) Send(msg []byte) error {
	if err := s.In.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("jsonrpc: write deadline: %w", err)
	}
	if _, err := s.In.Write(append(msg, '\n')); err != nil {
		return fmt.Errorf("jsonrpc: write: %w", err)
	}
	return nil
}

// Recv skips blank lines; a last line without its newline is still delivered.
func (s *stdioTransport) Recv() ([]byte, error) {
	for {
		line, err := s.lines.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			return line, nil
		}
		if err != nil {
			return nil, fmt.Errorf("jsonrpc: read: %w", err)
		}
	}
}
