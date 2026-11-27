package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

const initializeTimeout = 30 * time.Second

type lookPathFunc func(
	command string,
) (string, error)

type SpawnFunc func(
	ctx context.Context,
	spec registry.ServerSpec,
	worktreePath string,
) (server.Server, error)

type Manager interface {
	ServerForFile(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) (server.Server, error)
	RunningServerForFile(
		wsID string,
		filePath string,
	) (server.Server, error)
	Status(
		wsID string,
		filePath string,
	) domlsp.ServerStatus
	Acquire(
		wsID string,
		languageID string,
	)
	Release(
		ctx context.Context,
		wsID string,
		languageID string,
	)
	ReleaseWorkspace(
		ctx context.Context,
		wsID string,
	)
	OnDiagnostics(
		fn func(domlsp.DiagnosticsEvent),
	)
	OnReleaseEmpty(
		fn func(wsID string),
	)
	Shutdown(
		ctx context.Context,
	)
}

type entry struct {
	srv  server.Server
	refs int
}

type manager struct {
	reg        registry.Registry
	spawn      SpawnFunc
	lookPath   lookPathFunc
	mu         sync.Mutex
	pool       map[string]*entry
	onDiag     func(domlsp.DiagnosticsEvent)
	onRelEmpty func(wsID string)
}

type Option func(
	m *manager,
)

func WithLookPath(
	lookPath func(command string) (string, error),
) Option {
	return func(
		m *manager,
	) {
		m.lookPath = lookPath
	}
}

func New(
	reg registry.Registry,
	spawn SpawnFunc,
	opts ...Option,
) Manager {
	m := &manager{
		reg:      reg,
		spawn:    spawn,
		lookPath: exec.LookPath,
		pool:     make(map[string]*entry),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func poolKey(
	wsID string,
	languageID string,
) string {
	return wsID + "|" + languageID
}

func (m *manager) OnDiagnostics(
	fn func(domlsp.DiagnosticsEvent),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDiag = fn
}

func (m *manager) OnReleaseEmpty(
	fn func(wsID string),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRelEmpty = fn
}

func (m *manager) ServerForFile(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
) (server.Server, error) {
	spec, ok := m.reg.ForFile(filePath)
	if !ok {
		return nil, fmt.Errorf("lsp manager: no server for file: %w", ErrNoServer)
	}
	return m.getOrSpawn(ctx, wsID, worktreePath, spec)
}

func (m *manager) RunningServerForFile(
	wsID string,
	filePath string,
) (server.Server, error) {
	spec, ok := m.reg.ForFile(filePath)
	if !ok {
		return nil, fmt.Errorf("lsp manager: no server for file: %w", ErrNoServer)
	}
	key := poolKey(wsID, spec.LanguageID)

	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.pool[key]
	if !ok {
		return nil, fmt.Errorf("lsp manager: no running server: %w", ErrNoServer)
	}
	return e.srv, nil
}

func (m *manager) Status(
	wsID string,
	filePath string,
) domlsp.ServerStatus {
	spec, ok := m.reg.ForFile(filePath)
	if !ok {
		return domlsp.ServerStatus{State: domlsp.ServerUnsupported}
	}
	status := domlsp.ServerStatus{LanguageID: spec.LanguageID, Command: spec.Command}

	m.mu.Lock()
	_, running := m.pool[poolKey(wsID, spec.LanguageID)]
	m.mu.Unlock()

	switch {
	case running:
		status.State = domlsp.ServerRunning
	case !m.installed(spec.Command):
		status.State = domlsp.ServerNotInstalled
	default:
		status.State = domlsp.ServerStopped
	}
	return status
}

func (m *manager) installed(
	command string,
) bool {
	_, err := m.lookPath(command)
	return err == nil
}

func (m *manager) getOrSpawn(
	ctx context.Context,
	wsID string,
	worktreePath string,
	spec registry.ServerSpec,
) (server.Server, error) {
	key := poolKey(wsID, spec.LanguageID)

	m.mu.Lock()
	if e, ok := m.pool[key]; ok {
		e.refs++
		m.mu.Unlock()
		return e.srv, nil
	}
	m.mu.Unlock()

	if _, err := m.lookPath(spec.Command); err != nil {
		return nil, fmt.Errorf("lsp manager: %s not installed: %w", spec.Command, ErrNoServer)
	}

	srv, err := m.spawnAndWire(ctx, wsID, spec, worktreePath)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if existing, ok := m.pool[key]; ok {
		m.mu.Unlock()
		closeServer(srv)
		existing.refs++
		return existing.srv, nil
	}
	m.pool[key] = &entry{srv: srv, refs: 1}
	m.mu.Unlock()

	return srv, nil
}

func (m *manager) spawnAndWire(
	ctx context.Context,
	wsID string,
	spec registry.ServerSpec,
	worktreePath string,
) (server.Server, error) {
	srv, err := m.spawn(ctx, spec, worktreePath)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("lsp manager: %s not installed: %w", spec.Command, ErrNoServer)
	}
	if err != nil {
		return nil, fmt.Errorf("lsp manager: spawn %s: %w", spec.Command, err)
	}

	m.mu.Lock()
	cb := m.onDiag
	m.mu.Unlock()

	if cb != nil {
		srv.OnDiagnostics(stampWorkspace(wsID, worktreePath, cb))
	}

	srv.OnExit(func() { m.onServerExit(wsID, spec.LanguageID) })

	initCtx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()
	if err := srv.Initialize(initCtx, worktreePath); err != nil {
		return nil, errors.Join(fmt.Errorf("lsp manager: initialize %s: %w", spec.Command, err), srv.Close())
	}
	return srv, nil
}

func stampWorkspace(
	wsID string,
	worktreePath string,
	cb func(domlsp.DiagnosticsEvent),
) func(domlsp.DiagnosticsEvent) {
	return func(
		event domlsp.DiagnosticsEvent,
	) {
		event.WsID = wsID
		event.Diagnostics = relDiagnostics(worktreePath, event.Diagnostics)
		cb(event)
	}
}

func relDiagnostics(
	worktreePath string,
	diagnostics []domlsp.Diagnostic,
) []domlsp.Diagnostic {
	for i := range diagnostics {
		diagnostics[i].FilePath = convert.WorkspaceRelPath(worktreePath, diagnostics[i].FilePath)
	}
	return diagnostics
}

func (m *manager) Acquire(
	wsID string,
	languageID string,
) {
	key := poolKey(wsID, languageID)
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.pool[key]
	if !ok {
		return
	}
	e.refs++
}

func (m *manager) Release(
	ctx context.Context,
	wsID string,
	languageID string,
) {
	key := poolKey(wsID, languageID)

	m.mu.Lock()
	e, ok := m.pool[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	e.refs--
	if e.refs > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.pool, key)
	srv := e.srv
	empty := !m.hasWorkspace(wsID)
	cb := m.onRelEmpty
	m.mu.Unlock()

	closeServer(srv)
	if empty && cb != nil {
		cb(wsID)
	}
}

func (m *manager) onServerExit(
	wsID string,
	languageID string,
) {
	key := poolKey(wsID, languageID)

	m.mu.Lock()
	e, ok := m.pool[key]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.pool, key)
	srv := e.srv
	empty := !m.hasWorkspace(wsID)
	cb := m.onRelEmpty
	m.mu.Unlock()

	closeServer(srv)
	if empty && cb != nil {
		cb(wsID)
	}
}

func (m *manager) ReleaseWorkspace(
	_ context.Context,
	wsID string,
) {
	prefix := wsID + "|"

	m.mu.Lock()
	var srvs []server.Server
	for key, e := range m.pool {
		if strings.HasPrefix(key, prefix) {
			srvs = append(srvs, e.srv)
			delete(m.pool, key)
		}
	}
	cb := m.onRelEmpty
	m.mu.Unlock()

	for _, srv := range srvs {
		closeServer(srv)
	}
	if len(srvs) > 0 && cb != nil {
		cb(wsID)
	}
}

func (m *manager) hasWorkspace(
	wsID string,
) bool {
	prefix := wsID + "|"
	for key := range m.pool {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (m *manager) Shutdown(
	_ context.Context,
) {
	m.mu.Lock()
	pool := m.pool
	m.pool = make(map[string]*entry)
	m.mu.Unlock()

	for _, e := range pool {
		closeServer(e.srv)
	}
}

func closeServer(srv server.Server) {
	if err := srv.Close(); err != nil {
		slog.Warn("lsp manager: close server", "err", err)
	}
}
