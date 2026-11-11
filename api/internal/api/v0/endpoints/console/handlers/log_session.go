package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/logring"
)

const (
	logWriteTimeout = 10 * time.Second
	logReadLimit    = 1024
)

type logSession struct {
	conn   *websocket.Conn
	stream logring.Stream
}

func newLogSession(
	conn *websocket.Conn,
	stream logring.Stream,
) *logSession {
	return &logSession{conn: conn, stream: stream}
}

func (s *logSession) run() {
	defer s.release()

	gone := s.watch()
	if s.replay() != nil {
		return
	}
	for s.follow(gone) {
	}
}

func (s *logSession) release() {
	s.stream.Close()
	libs.CloseLogged(context.Background(), "console: close the log socket", s.conn)
}

func (s *logSession) replay() error {
	err := s.sendAll(s.stream.Replay())
	if err != nil {
		return err
	}
	ready := map[string]any{"type": "ready", "seq": s.stream.Seq()}
	if s.stream.Reset() {
		ready["reset"] = true
	}
	return s.send(ready)
}

func (s *logSession) sendAll(
	records []logring.Record,
) error {
	var err error
	for i := 0; i < len(records) && err == nil; i++ {
		err = s.send(records[i])
	}
	return err
}

func (s *logSession) follow(
	gone <-chan struct{},
) bool {
	select {
	case rec, open := <-s.stream.Live():
		return open && s.send(rec) == nil
	case <-gone:
		return false
	}
}

func (s *logSession) send(
	frame any,
) error {
	if err := s.conn.SetWriteDeadline(time.Now().Add(logWriteTimeout)); err != nil {
		return fmt.Errorf("console: log socket: write deadline: %w", err)
	}
	return s.conn.WriteJSON(frame)
}

func (s *logSession) watch() <-chan struct{} {
	gone := make(chan struct{})
	s.conn.SetReadLimit(logReadLimit)
	go s.discardUntilClosed(gone)
	return gone
}

func (s *logSession) discardUntilClosed(
	gone chan<- struct{},
) {
	defer close(gone)
	for s.discardNext() {
	}
}

func (s *logSession) discardNext() bool {
	_, _, err := s.conn.NextReader()
	return err == nil
}
