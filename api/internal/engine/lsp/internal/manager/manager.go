// Package manager maintains a per-workspace pool of language-server processes.
// Each (wsID, languageID) pair has at most one running server. Servers are
// spawned lazily on first use and kept alive by an independent refcount;
// the last Release tears down the server (10 §5).
package manager

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

// initializeTimeout bounds the LSP initialize handshake, which can be slow on a
// language server's cold start (module loading, indexing).
const initializeTimeout = 30 * time.Second

// lookPathFunc resolves a command name to an absolute path, mirroring
// exec.LookPath. Tests inject a fake to simulate a missing binary.
type lookPathFunc func(
	command string,
) (string, error)

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
	// RunningServerForFile returns the already-running server for filePath's
	// language WITHOUT spawning and WITHOUT touching the refcount. It returns
	// ErrNoServer when the registry has no spec for the file or no server is
	// currently running for (wsID, languageID). The transient document-close path
	// uses this to forward textDocument/didClose on the ref it already holds from
	// the matching didOpen, so closing does not itself acquire a new ref.
	RunningServerForFile(
		wsID string,
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
	// ReleaseWorkspace closes and removes every server running for wsID
	// regardless of refcount. It is the engine-side release for the workspace WS
	// topic's last-unsubscribe edge, covering a force-quit/crash/WS-drop where
	// the per-document Release-to-zero path never runs.
	ReleaseWorkspace(
		ctx context.Context,
		wsID string,
	)
	// OnDiagnostics registers the callback that receives diagnostics events from
	// every server in the pool, present and future.
	OnDiagnostics(
		fn func(domlsp.DiagnosticsEvent),
	)
	// OnReleaseEmpty registers the callback invoked when a Release drops the last
	// running server for a workspace, so callers can evict per-workspace state.
	OnReleaseEmpty(
		fn func(wsID string),
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
	reg        registry.Registry
	spawn      SpawnFunc
	lookPath   lookPathFunc
	mu         sync.Mutex
	pool       map[string]*entry
	onDiag     func(domlsp.DiagnosticsEvent)
	onRelEmpty func(wsID string)
}

// Option customizes a Manager at construction. Options are an internal seam:
// production code passes none and gets exec.LookPath binary resolution; tests
// inject a fake LookPath to simulate present or missing server binaries.
type Option func(
	m *manager,
)

// WithLookPath overrides the exec.LookPath used to verify a server binary is
// installed before spawning. Intended for tests.
func WithLookPath(
	lookPath func(command string) (string, error),
) Option {
	return func(
		m *manager,
	) {
		m.lookPath = lookPath
	}
}

// New returns a Manager backed by reg and spawn. Pass a nil SpawnFunc only in
// tests that never call ServerForFile. The server binary is resolved on PATH
// via exec.LookPath before spawning; a missing binary yields ErrNoServer.
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
		srv.OnDiagnostics(stampWsID(wsID, cb))
	}

	// Reap-and-evict: when the server process exits on its own (crash/OOM), the
	// transport reaper fires this so the dead pool entry is removed and the next
	// request respawns a live server instead of blocking on the corpse to the
	// 10s request timeout (R10). A deliberate Close (Release/Shutdown/Replay)
	// does NOT fire it.
	srv.OnExit(func() { m.onServerExit(wsID, spec.LanguageID) })

	// The LSP handshake must complete before any document sync, and gopls cold
	// starts can take several seconds, so give it its own timeout rather than
	// inheriting a short request deadline.
	initCtx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()
	if err := srv.Initialize(initCtx, worktreePath); err != nil {
		_ = srv.Close()
		return nil, fmt.Errorf("lsp manager: initialize %s: %w", spec.Command, err)
	}
	return srv, nil
}

func stampWsID(
	wsID string,
	cb func(domlsp.DiagnosticsEvent),
) func(domlsp.DiagnosticsEvent) {
	return func(
		event domlsp.DiagnosticsEvent,
	) {
		event.WsID = wsID
		cb(event)
	}
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

	_ = srv.Close()
	if empty && cb != nil {
		cb(wsID)
	}
}

// onServerExit evicts the pool entry for a server whose process exited on its
// own. The transport has already reaped itself; closing the server here fails
// any in-flight request waiters so they return immediately instead of blocking
// to the request timeout. A re-spawn is left to the next ServerForFile. If this
// was the workspace's last server, the empty callback fires so per-workspace
// state (the diagnostics snapshot) is evicted, mirroring Release.
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

	_ = srv.Close()
	if empty && cb != nil {
		cb(wsID)
	}
}

// ReleaseWorkspace closes and removes every server running for wsID regardless
// of refcount, then fires the empty callback once if any server was torn down.
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
		_ = srv.Close()
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
		_ = e.srv.Close()
	}
}
