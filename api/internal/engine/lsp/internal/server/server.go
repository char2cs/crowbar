package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
)

const methodPublishDiagnostics = "textDocument/publishDiagnostics"

var ErrClosed = errors.New("server: closed")

var ErrNoSpawn = errors.New("server: no spawn closure")

var ErrReplaced = errors.New("server: transport replaced")

type spawnFunc func(ctx context.Context) (io.ReadWriteCloser, error)

type waiterResult struct {
	resp protocol.Response
	err  error
}

type Server interface {
	Request(
		ctx context.Context,
		method string,
		params any,
	) (json.RawMessage, error)
	Notify(
		ctx context.Context,
		method string,
		params any,
	) error
	Initialize(
		ctx context.Context,
		rootDir string,
	) error
	OnDiagnostics(
		fn func(lsp.DiagnosticsEvent),
	)
	OnExit(
		fn func(),
	)
	OpenDocs() *OpenDocs
	Replay(
		ctx context.Context,
	) error
	SemanticTokens() semtok.Support
	CanExecute(
		command string,
	) bool
	ExecuteCommand(
		ctx context.Context,
		params any,
	) (json.RawMessage, []json.RawMessage, error)
	Close() error
}

type server struct {
	spawn spawnFunc

	mu           sync.Mutex
	transport    io.ReadWriteCloser
	reader       *bufio.Reader
	nextID       int
	waiters      map[int]chan waiterResult
	onDiag       func(lsp.DiagnosticsEvent)
	onExit       func()
	closed       bool
	rootDir      string
	openParams   map[string]json.RawMessage
	initOptions  map[string]any
	features     serverFeatures
	commandEdits *[]json.RawMessage
	execMu       sync.Mutex

	writeMu sync.Mutex
	docs    *OpenDocs
}

func newServer(
	initOptions map[string]any,
) *server {
	return &server{
		waiters:     make(map[int]chan waiterResult),
		openParams:  make(map[string]json.RawMessage),
		docs:        NewOpenDocs(),
		initOptions: initOptions,
	}
}

func New(
	ctx context.Context,
	command string,
	args []string,
	dir string,
	initOptions map[string]any,
) (Server, error) {
	s := newServer(initOptions)
	s.spawn = commandSpawn(command, args, dir, s.handleProcessExit)
	transport, err := s.spawn(ctx)
	if err != nil {
		return nil, err
	}
	s.swapTransport(transport)
	return s, nil
}

func commandSpawn(
	command string,
	args []string,
	dir string,
	onExit func(),
) spawnFunc {
	return func(
		_ context.Context,
	) (io.ReadWriteCloser, error) {
		cmd := exec.Command(command, args...) // #nosec G204 G702
		cmd.Dir = dir
		return newProcessTransport(cmd, onExit)
	}
}

func (s *server) OpenDocs() *OpenDocs {
	return s.docs
}

func (s *server) Initialize(
	ctx context.Context,
	rootDir string,
) error {
	s.mu.Lock()
	s.rootDir = rootDir
	s.mu.Unlock()
	return s.handshake(ctx, rootDir)
}

func (s *server) handshake(
	ctx context.Context,
	rootDir string,
) error {
	rootURI := convert.URIFromPath(rootDir)
	params := map[string]any{
		"processId":        os.Getpid(),
		"rootUri":          rootURI,
		"capabilities":     clientCapabilities(),
		"workspaceFolders": []any{map[string]any{"uri": rootURI, "name": "root"}},
	}
	if len(s.initOptions) > 0 {
		params["initializationOptions"] = s.initOptions
	}
	result, err := s.Request(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := s.recordFeatures(result); err != nil {
		return err
	}
	return s.Notify(ctx, "initialized", map[string]any{})
}

func (s *server) SemanticTokens() semtok.Support {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.features.semTok
}

func (s *server) CanExecute(
	command string,
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.features.commands[command]
}

func (s *server) OnDiagnostics(
	fn func(lsp.DiagnosticsEvent),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDiag = fn
}

func (s *server) OnExit(
	fn func(),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onExit = fn
}

func (s *server) handleProcessExit() {
	s.mu.Lock()
	fn := s.onExit
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
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
	ch chan waiterResult,
) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("request: ctx: %w", ctx.Err())
	case res, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		if res.err != nil {
			return nil, res.err
		}
		if res.resp.Error != nil {
			return nil, fmt.Errorf("request: rpc error %d: %s", res.resp.Error.Code, res.resp.Error.Message)
		}
		return res.resp.Result, nil
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

func (s *server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	transport := s.transport
	waiters := s.waiters
	s.waiters = make(map[int]chan waiterResult)
	s.mu.Unlock()

	failWaiters(waiters, ErrClosed)
	return transport.Close()
}

func failWaiters(
	waiters map[int]chan waiterResult,
	err error,
) {
	for _, ch := range waiters {
		ch <- waiterResult{err: err}
		close(ch)
	}
}

func (s *server) register() (
	int,
	chan waiterResult,
	error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, nil, ErrClosed
	}
	s.nextID++
	id := s.nextID
	ch := make(chan waiterResult, 1)
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
	waiters := s.waiters
	s.waiters = make(map[int]chan waiterResult)
	s.mu.Unlock()

	failWaiters(waiters, ErrReplaced)
	if old != nil {
		closeLogged("close replaced transport", old)
	}
	go s.readLoop(transport, reader)
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
