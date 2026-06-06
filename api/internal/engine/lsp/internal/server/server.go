// Package server runs and proxies a single language-server process. It speaks
// JSON-RPC over stdio, correlates request/response by id, dispatches
// publishDiagnostics notifications to a callback, and replays didOpen for every
// tracked URI when the process is respawned (10 §3, §5).
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

const methodPublishDiagnostics = "textDocument/publishDiagnostics"

// ErrClosed is returned by operations on a server that has been closed.
var ErrClosed = errors.New("server: closed")

// ErrNoSpawn is returned by Replay when the server was created without a spawn
// closure (a test transport with no respawn support).
var ErrNoSpawn = errors.New("server: no spawn closure")

// spawnFunc establishes a fresh transport to a (re)spawned language server.
type spawnFunc func(ctx context.Context) (io.ReadWriteCloser, error)

// Server is one running language-server process proxied over JSON-RPC.
type Server interface {
	// Request sends a JSON-RPC request and waits for the correlated response or
	// ctx cancellation.
	Request(
		ctx context.Context,
		method string,
		params any,
	) (json.RawMessage, error)
	// Notify sends a JSON-RPC notification (no response expected). didOpen and
	// didClose notifications update the tracked open-URI set.
	Notify(
		ctx context.Context,
		method string,
		params any,
	) error
	// OnDiagnostics registers the callback invoked for every publishDiagnostics
	// notification.
	OnDiagnostics(
		fn func(lsp.DiagnosticsEvent),
	)
	// OpenDocs returns the content-free set of currently open document URIs.
	OpenDocs() *openDocs
	// Replay respawns the underlying process and re-sends didOpen for every
	// tracked URI.
	Replay(
		ctx context.Context,
	) error
	// Close terminates the process and fails any in-flight requests.
	Close() error
}

type server struct {
	spawn spawnFunc

	mu        sync.Mutex
	transport io.ReadWriteCloser
	reader    *bufio.Reader
	nextID    int
	waiters   map[int]chan protocol.Response
	onDiag    func(lsp.DiagnosticsEvent)
	closed    bool

	writeMu sync.Mutex
	docs    *openDocs
}

func newOverTransport(
	transport io.ReadWriteCloser,
	spawn spawnFunc,
) Server {
	s := &server{
		spawn:     spawn,
		transport: transport,
		reader:    bufio.NewReader(transport),
		waiters:   make(map[int]chan protocol.Response),
		docs:      newOpenDocs(),
	}
	go s.readLoop(s.transport, s.reader)
	return s
}

// New spawns the language server described by command/args in dir and returns a
// running Server. The process stdin/stdout become the JSON-RPC transport.
func New(
	command string,
	args []string,
	dir string,
) (Server, error) {
	spawn := commandSpawn(command, args, dir)
	transport, err := spawn(context.Background())
	if err != nil {
		return nil, err
	}
	s := &server{
		spawn:     spawn,
		transport: transport,
		reader:    bufio.NewReader(transport),
		waiters:   make(map[int]chan protocol.Response),
		docs:      newOpenDocs(),
	}
	go s.readLoop(s.transport, s.reader)
	return s, nil
}

func commandSpawn(
	command string,
	args []string,
	dir string,
) spawnFunc {
	return func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		// The command and args come from the trusted server registry, not from
		// untrusted input; launching the configured language server is the
		// engine's entire purpose.
		cmd := exec.Command(command, args...) // #nosec G204 G702
		cmd.Dir = dir
		return newProcessTransport(cmd)
	}
}

func (s *server) OpenDocs() *openDocs {
	return s.docs
}

func (s *server) OnDiagnostics(
	fn func(lsp.DiagnosticsEvent),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDiag = fn
}

func (s *server) Request(
	ctx context.Context,
	method string,
	params any,
) (json.RawMessage, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}

	id, ch, err := s.register()
	if err != nil {
		return nil, err
	}
	defer s.unregister(id)

	req := protocol.Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  raw,
	}
	if err := s.write(req); err != nil {
		return nil, err
	}
	return s.await(ctx, ch)
}

func (s *server) await(
	ctx context.Context,
	ch chan protocol.Response,
) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("request: ctx: %w", ctx.Err())
	case resp, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("request: rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (s *server) Notify(
	ctx context.Context,
	method string,
	params any,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("notify: ctx: %w", err)
	}
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	s.trackDoc(method, raw)

	notif := protocol.Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
	}
	return s.write(notif)
}

func (s *server) Replay(
	ctx context.Context,
) error {
	if s.spawn == nil {
		return ErrNoSpawn
	}
	transport, err := s.spawn(ctx)
	if err != nil {
		return fmt.Errorf("replay: spawn: %w", err)
	}
	s.swapTransport(transport)

	for _, uri := range s.docs.List() {
		params := map[string]any{
			"textDocument": map[string]any{"uri": uri},
		}
		if err := s.Notify(ctx, "textDocument/didOpen", params); err != nil {
			return fmt.Errorf("replay: didOpen %s: %w", uri, err)
		}
	}
	return nil
}

func (s *server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	transport := s.transport
	waiters := s.waiters
	s.waiters = make(map[int]chan protocol.Response)
	s.mu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}
	return transport.Close()
}

func (s *server) register() (
	int,
	chan protocol.Response,
	error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, nil, ErrClosed
	}
	s.nextID++
	id := s.nextID
	ch := make(chan protocol.Response, 1)
	s.waiters[id] = ch
	return id, ch, nil
}

func (s *server) unregister(
	id int,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.waiters, id)
}

func (s *server) write(
	payload any,
) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("write: marshal: %w", err)
	}

	s.mu.Lock()
	closed := s.closed
	transport := s.transport
	s.mu.Unlock()
	if closed {
		return ErrClosed
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := protocol.WriteMessage(transport, raw); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (s *server) swapTransport(
	transport io.ReadWriteCloser,
) {
	reader := bufio.NewReader(transport)

	s.mu.Lock()
	old := s.transport
	s.transport = transport
	s.reader = reader
	s.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	go s.readLoop(transport, reader)
}

func (s *server) trackDoc(
	method string,
	params json.RawMessage,
) {
	if method != "textDocument/didOpen" && method != "textDocument/didClose" {
		return
	}
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	if p.TextDocument.URI == "" {
		return
	}
	if method == "textDocument/didOpen" {
		s.docs.Add(p.TextDocument.URI)
		return
	}
	s.docs.Remove(p.TextDocument.URI)
}

func marshalParams(
	params any,
) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("params: marshal: %w", err)
	}
	return raw, nil
}
