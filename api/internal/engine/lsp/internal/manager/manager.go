// Package manager maintains a per-workspace pool of language-server processes.
// Each (wsID, languageID) pair has at most one running server. Servers are
// spawned lazily on first use and kept alive by an independent refcount;
// the last Release tears down the server (10 §5).
package manager

import (
	"context"
	"fmt"
	"sync"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

// SpawnFunc creates a running language-server process for the given spec in the
// given worktree directory. Tests inject a fake to avoid real binaries.
type SpawnFunc func(
	ctx context.Context,
	spec registry.ServerSpec,
	worktreePath string,
) (server.Server, error)

// Manager maintains a pool of language servers keyed by (wsID, languageID).
// All methods are safe for concurrent use.
type Manager interface {
	// ServerForFile resolves the spec for filePath from the registry, spawns a
	// server for (wsID, languageID) if none is running, increments its refcount,
	// and returns it. Returns ErrNoServer when no spec covers the extension.
	ServerForFile(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) (server.Server, error)
	// Acquire increments the refcount for the (wsID, languageID) entry without
	// spawning. It is a no-op when no entry exists.
	Acquire(
		wsID string,
		languageID string,
	)
	// Release decrements the refcount for (wsID, languageID). When the count
	// reaches zero the server is closed and removed from the pool.
	Release(
		ctx context.Context,
		wsID string,
		languageID string,
	)
	// OnDiagnostics registers the callback that receives diagnostics events from
	// every server in the pool, present and future.
	OnDiagnostics(
		fn func(domlsp.DiagnosticsEvent),
	)
	// Shutdown closes all running servers regardless of refcount.
	Shutdown(
		ctx context.Context,
	)
}

type entry struct {
	srv  server.Server
	refs int
}

type manager struct {
	reg    registry.Registry
	spawn  SpawnFunc
	mu     sync.Mutex
	pool   map[string]*entry
	onDiag func(domlsp.DiagnosticsEvent)
}

// New returns a Manager backed by reg and spawn. Pass a nil SpawnFunc only in
// tests that never call ServerForFile.
func New(
	reg registry.Registry,
	spawn SpawnFunc,
) Manager {
	return &manager{
		reg:   reg,
		spawn: spawn,
		pool:  make(map[string]*entry),
	}
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

	srv, err := m.spawnAndWire(ctx, spec, worktreePath)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	// Re-check after the lock gap: another goroutine may have raced us.
	if existing, ok := m.pool[key]; ok {
		m.mu.Unlock()
		_ = srv.Close()
		existing.refs++
		return existing.srv, nil
	}
	m.pool[key] = &entry{srv: srv, refs: 1}
	m.mu.Unlock()

	return srv, nil
}

func (m *manager) spawnAndWire(
	ctx context.Context,
	spec registry.ServerSpec,
	worktreePath string,
) (server.Server, error) {
	srv, err := m.spawn(ctx, spec, worktreePath)
	if err != nil {
		return nil, fmt.Errorf("lsp manager: spawn %s: %w", spec.Command, err)
	}

	m.mu.Lock()
	cb := m.onDiag
	m.mu.Unlock()

	if cb != nil {
		srv.OnDiagnostics(cb)
	}
	return srv, nil
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
	m.mu.Unlock()

	_ = srv.Close()
}

func (m *manager) Shutdown(
	_ context.Context,
) {
	m.mu.Lock()
	pool := m.pool
	m.pool = make(map[string]*entry)
	m.mu.Unlock()

	for _, e := range pool {
		_ = e.srv.Close()
	}
}
