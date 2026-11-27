package jsonrpc

import (
	"bufio"
	"bytes"
	"fmt"
	"time"
)

type Transport interface {
	Send([]byte) error
	Recv() ([]byte, error)
	Close() error
}

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
