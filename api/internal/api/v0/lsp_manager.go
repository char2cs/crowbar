package v0

import (
	"context"
	"sync"
)

// LSPManager is the refcounted, lazy lifecycle registry for per-workspace LSP
// hosts (03 §6). Its refcount is INDEPENDENT of the WatcherManager's: it counts
// subscribers to the LSP topic only. The first LSP subscriber for a workspace
// runs Ensure; the last runs Shutdown. An LSP-only subscriber must not start
// the file watcher, and a Files/Git subscriber must not touch LSP — the two
// managers share no state, which is what enforces that independence.
type LSPManager struct {
	root      context.Context
	lifecycle lspLifecycle
	mu        sync.Mutex
	refs      map[string]int
	closed    bool
}

// NewLSPManager builds an LSPManager over the given lifecycle and a root
// context passed to Ensure/Shutdown.
func NewLSPManager(
	root context.Context,
	lifecycle lspLifecycle,
) *LSPManager {
	return &LSPManager{
		root:      root,
		lifecycle: lifecycle,
		refs:      make(map[string]int),
	}
}

// Acquire records one LSP subscriber for wsID, running Ensure on the 0→1 edge.
// After StopAll has run it is a no-op, so a late subscribe from a not-yet-closed
// hijacked WS connection cannot start an LSP host under the cancelled root.
func (m *LSPManager) Acquire(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.refs[wsID]++
	first := m.refs[wsID] == 1
	m.mu.Unlock()

	if !first {
		return
	}
	m.lifecycle.Ensure(m.root, wsID)
}

// Release drops one LSP subscriber for wsID, running Shutdown on the 1→0 edge.
func (m *LSPManager) Release(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	if m.refs[wsID] == 0 {
		m.mu.Unlock()
		return
	}
	m.refs[wsID]--
	last := m.refs[wsID] == 0
	if last {
		delete(m.refs, wsID)
	}
	m.mu.Unlock()

	if !last {
		return
	}
	m.lifecycle.Shutdown(m.root, wsID)
}

// StopAll runs Shutdown for every workspace the manager currently holds and
// clears the refcount map. It is idempotent and safe to call when nothing is
// held: a manager holding nothing is a no-op. It runs on graceful shutdown so
// LSP host processes are torn down promptly rather than waiting on process exit.
func (m *LSPManager) StopAll() {
	m.mu.Lock()
	m.closed = true
	wsIDs := make([]string, 0, len(m.refs))
	for wsID := range m.refs {
		wsIDs = append(wsIDs, wsID)
	}
	m.refs = make(map[string]int)
	m.mu.Unlock()

	for _, wsID := range wsIDs {
		m.lifecycle.Shutdown(m.root, wsID)
	}
}
