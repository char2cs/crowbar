package v0

import (
	"context"
	"sync"
)

// watcherHandle tracks one workspace's running watcher: its subscriber
// refcount and the cancel func that stops the Start goroutine.
type watcherHandle struct {
	refs   int
	cancel context.CancelFunc
	proc   watcherProc
}

// WatcherManager is the refcounted, lazy lifecycle registry for per-workspace
// file watchers (03 §6). The refcount spans the Files AND Git topics together:
// both broadcasters' OnSubscribe/OnUnsubscribe call this same manager keyed by
// wsID, so the watcher starts on the first (Files ∪ Git) subscriber and stops
// on the last. A git-only sidebar subscriber therefore keeps the watcher alive,
// because the watcher is what produces GitStatus and the SyncWorkingTreeState
// command.
type WatcherManager struct {
	root    context.Context
	factory watcherFactory
	mu      sync.Mutex
	handles map[string]*watcherHandle
	closed  bool
}

// NewWatcherManager builds a WatcherManager. root is the parent context for
// every watcher goroutine; cancelling it stops all watchers.
func NewWatcherManager(
	root context.Context,
	factory watcherFactory,
) *WatcherManager {
	return &WatcherManager{
		root:    root,
		factory: factory,
		handles: make(map[string]*watcherHandle),
	}
}

// Acquire records one subscriber for wsID. On the 0→1 transition it builds and
// starts the watcher in a goroutine under a cancelable context. A blank wsID is
// ignored so a misrouted subscribe can never start an unscoped watcher. After
// StopAll has run it is a no-op, so a late subscribe from a not-yet-closed
// hijacked WS connection cannot start a watcher under the cancelled root.
func (m *WatcherManager) Acquire(
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
	if h, ok := m.handles[wsID]; ok {
		h.refs++
		m.mu.Unlock()
		return
	}
	proc, err := m.factory(m.root, wsID)
	if err != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.root)
	m.handles[wsID] = &watcherHandle{refs: 1, cancel: cancel, proc: proc}
	m.mu.Unlock()

	go func() {
		_ = proc.Start(ctx)
	}()
}

// Release drops one subscriber for wsID. On the 1→0 transition it cancels the
// watcher goroutine and stops the watcher.
func (m *WatcherManager) Release(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	h, ok := m.handles[wsID]
	if !ok {
		m.mu.Unlock()
		return
	}
	h.refs--
	if h.refs > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.handles, wsID)
	m.mu.Unlock()

	h.cancel()
	h.proc.Stop()
}

// StopAll cancels and stops every live watcher and clears the handle map. It is
// idempotent and safe to call when no watcher is running: a manager holding
// nothing is a no-op. It runs on graceful shutdown so inotify file descriptors
// are closed promptly rather than waiting on process exit.
func (m *WatcherManager) StopAll() {
	m.mu.Lock()
	m.closed = true
	handles := m.handles
	m.handles = make(map[string]*watcherHandle)
	m.mu.Unlock()

	for _, h := range handles {
		h.cancel()
		h.proc.Stop()
	}
}
