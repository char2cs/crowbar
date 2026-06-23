// Package terminal is the PTY session engine. It manages session lifecycle,
// ring-buffer replay, and multi-client WebSocket fan-out.
package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/profile"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// WSConn is the WebSocket abstraction implemented by gorilla/websocket connections.
type WSConn interface {
	WriteMessage(
		messageType int,
		data []byte,
	) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

// ErrSessionNotFound is returned when an operation targets a session ID that
// does not exist in the registry.
var ErrSessionNotFound = registry.ErrSessionNotFound

// Engine is the full PTY session operation surface.
type Engine interface {
	// Create spawns a new PTY session in the given workspace directory.
	Create(
		ctx context.Context,
		workspaceID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)

	// Attach connects a WebSocket connection to an existing session,
	// replaying the ring buffer and streaming live output.
	// The conn is closed by the engine when the session ends or the client is dropped.
	Attach(
		ctx context.Context,
		sessionID string,
		conn WSConn,
	) error

	// Write sends input bytes to the session's PTY stdin.
	Write(
		ctx context.Context,
		sessionID string,
		data []byte,
	) error

	// Resize sends SIGWINCH with the new terminal dimensions.
	Resize(
		ctx context.Context,
		sessionID string,
		cols uint16,
		rows uint16,
	) error

	// Kill terminates the session and deregisters it.
	Kill(
		ctx context.Context,
		sessionID string,
	) error

	// ListSessions returns all active session IDs.
	ListSessions() []string

	// ListSessionsForWorkspace returns the active session IDs owned by the given
	// workspace.
	ListSessionsForWorkspace(
		workspaceID string,
	) []string

	// OnSessionEnded registers the callback invoked when a session terminates
	// (a Kill, a Shutdown, or a PTY self-exit). It fires exactly once per
	// session so the lifecycle topic can emit an "ended" frame.
	OnSessionEnded(
		fn func(ctx context.Context, workspaceID string, sessionID string),
	)

	// SessionExists reports whether a session with the given ID is currently active.
	SessionExists(
		ctx context.Context,
		sessionID string,
	) bool

	// Shutdown terminates all active sessions and removes them from the registry.
	Shutdown()
}

type terminalEngine struct {
	reg *registry.Registry

	mu        sync.RWMutex
	onEnded   func(ctx context.Context, workspaceID string, sessionID string)
	endedOnce map[string]struct{}
}

// New returns a new Engine.
func New() Engine {
	return &terminalEngine{
		reg:       registry.New(),
		endedOnce: make(map[string]struct{}),
	}
}

var _ Engine = (*terminalEngine)(nil)

// outputMsg is the server→client wire frame.
type outputMsg struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
	IsInput   bool   `json:"isInput"`
}

// inputMsg is the client→server wire frame for PTY input.
type inputMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (e *terminalEngine) Create(
	_ context.Context,
	workspaceID string,
	workspaceDir string,
	prof *domain.TerminalProfile,
) (string, error) {
	resolved := profile.Resolve(prof, workspaceDir)
	id := uuid.NewString()

	s, err := session.New(
		id,
		resolved.Shell,
		resolved.CWD,
		os.Environ(),
	)
	if err != nil {
		return "", fmt.Errorf("terminal: create: %w", err)
	}

	for _, cmd := range resolved.Startup {
		if writeErr := s.Write([]byte(cmd + "\n")); writeErr != nil {
			// Non-fatal: PTY is alive but startup command failed.
			break
		}
	}

	e.reg.Add(id, workspaceID, s)

	go e.reapOnDone(id, workspaceID, s)
	return id, nil
}

// reapOnDone removes the session from the registry once it terminates and fires
// the OnSessionEnded callback so the lifecycle topic can emit an "ended" frame.
// It covers every termination path: an explicit Kill, a Shutdown, or a PTY
// self-exit.
func (e *terminalEngine) reapOnDone(
	id string,
	workspaceID string,
	s *session.Session,
) {
	<-s.Done()
	e.reg.Remove(id)
	e.fireEnded(context.Background(), workspaceID, id)
}

// fireEnded invokes the registered OnSessionEnded callback exactly once per
// session id, guarding against duplicate notifications.
func (e *terminalEngine) fireEnded(
	ctx context.Context,
	workspaceID string,
	sessionID string,
) {
	e.mu.Lock()
	fn := e.onEnded
	if fn == nil {
		e.mu.Unlock()
		return
	}
	if _, done := e.endedOnce[sessionID]; done {
		e.mu.Unlock()
		return
	}
	e.endedOnce[sessionID] = struct{}{}
	e.mu.Unlock()

	fn(ctx, workspaceID, sessionID)
}

// OnSessionEnded registers the termination callback. The most recent
// registration wins, mirroring the LSP engine's OnDiagnostics hook.
func (e *terminalEngine) OnSessionEnded(
	fn func(ctx context.Context, workspaceID string, sessionID string),
) {
	e.mu.Lock()
	e.onEnded = fn
	e.mu.Unlock()
}

func (e *terminalEngine) Attach(
	ctx context.Context,
	sessionID string,
	conn WSConn,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: attach: %w: %s", registry.ErrSessionNotFound, sessionID)
	}

	ch, err := s.Attach()
	if err != nil {
		return fmt.Errorf("terminal: attach: %w", err)
	}

	writeDone := make(chan struct{})
	go e.writePump(conn, sessionID, ch, writeDone)
	e.readPump(ctx, conn, s)

	s.Detach(ch)
	<-writeDone
	return nil
}

// writePump reads from ch and forwards output frames to the WebSocket.
// When the write fails or ch closes, it closes the connection so readPump unblocks.
func (e *terminalEngine) writePump(
	conn WSConn,
	sessionID string,
	ch <-chan session.OutputFrame,
	done chan<- struct{},
) {
	defer func() {
		_ = conn.Close()
		close(done)
	}()
	for frame := range ch {
		msg := outputMsg{
			SessionID: sessionID,
			Data:      string(frame.Data),
		}
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if writeErr := conn.WriteMessage(websocket.TextMessage, data); writeErr != nil {
			return
		}
	}
}

// readPump reads client messages from the WebSocket and dispatches them.
func (e *terminalEngine) readPump(
	_ context.Context,
	conn WSConn,
	s *session.Session,
) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg inputMsg
		if jsonErr := json.Unmarshal(raw, &msg); jsonErr != nil {
			continue
		}

		if msg.Type == "resize" {
			_ = s.Resize(msg.Cols, msg.Rows)
			continue
		}

		_ = s.Write([]byte(msg.Data))
	}
}

func (e *terminalEngine) Write(
	_ context.Context,
	sessionID string,
	data []byte,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: write: %w: %s", registry.ErrSessionNotFound, sessionID)
	}
	return s.Write(data)
}

func (e *terminalEngine) Resize(
	_ context.Context,
	sessionID string,
	cols uint16,
	rows uint16,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: resize: %w: %s", registry.ErrSessionNotFound, sessionID)
	}
	return s.Resize(cols, rows)
}

func (e *terminalEngine) Kill(
	_ context.Context,
	sessionID string,
) error {
	s, ok := e.reg.Get(sessionID)
	if !ok {
		return fmt.Errorf("terminal: kill: %w: %s", registry.ErrSessionNotFound, sessionID)
	}
	s.Kill()
	e.reg.Remove(sessionID)
	return nil
}

func (e *terminalEngine) ListSessions() []string {
	return e.reg.List()
}

// ListSessionsForWorkspace returns the active session IDs owned by workspaceID.
func (e *terminalEngine) ListSessionsForWorkspace(
	workspaceID string,
) []string {
	return e.reg.ListByWorkspace(workspaceID)
}

// SessionExists reports whether a session with the given ID is currently active.
func (e *terminalEngine) SessionExists(
	_ context.Context,
	sessionID string,
) bool {
	_, ok := e.reg.Get(sessionID)
	return ok
}

// Shutdown terminates all active sessions and removes them from the registry.
func (e *terminalEngine) Shutdown() {
	for _, id := range e.reg.List() {
		s, ok := e.reg.Get(id)
		if !ok {
			continue
		}
		s.Kill()
		e.reg.Remove(id)
	}
}
