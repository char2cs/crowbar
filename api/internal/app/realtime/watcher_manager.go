package realtime

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// watcherLingerDuration is the grace period a watcher stays alive after its last
// subscriber leaves. A files/ws reconnect storm (the app backgrounding, WS
// flapping) unsubscribes then resubscribes within this window; lingering lets
// the re-Acquire reuse the live watcher instead of tearing down and rebuilding
// it — the rebuild being the ~458-fork recursive walk this fix eliminates. It is
// long enough to swallow realistic reconnect backoff yet short enough that a
// genuinely closed workspace promptly releases its inotify FDs.
const watcherLingerDuration = 10 * time.Second

// watcherHandle tracks one workspace's running watcher: its subscriber refcount,
// the cancel func that stops the Start goroutine, the pending linger timer (non
// nil only while the watcher is lingering after its last release), and a
// generation counter that invalidates a linger timer whose callback fires after
// a reconnect or a fresh release cycle has superseded it.
type watcherHandle struct {
	refs   int
	cancel context.CancelFunc
	proc   watcherProc
	timer  lingerTimer
	gen    uint64
}

// WatcherManager is the refcounted, lazy lifecycle registry for per-workspace
// file watchers (03 §6). The refcount spans the Files AND Git topics together:
// both broadcasters' OnSubscribe/OnUnsubscribe call this same manager keyed by
// wsID, so the watcher starts on the first (Files ∪ Git) subscriber and stops
// on the last. A git-only sidebar subscriber therefore keeps the watcher alive,
// because the watcher is what produces GitStatus and the SyncWorkingTreeState
// command.
type WatcherManager struct {
	root     context.Context
	factory  watcherFactory
	linger   time.Duration
	newTimer timerFactory
	mu       sync.Mutex
	handles  map[string]*watcherHandle
	closed   bool
}

// NewWatcherManager builds the production WatcherManager from its dependencies.
// It wires the dispatcher (hub fan-out plus the SyncWorkingTreeState command on
// the workspace repository), the git status provider (adapting the git engine),
// and the watcher factory (resolving worktree paths via the workspace repository
// and constructing fs watchers). root is the parent context for every watcher
// goroutine; cancelling it stops all watchers. now is the clock threaded into
// the SyncWorkingTreeState command.
func NewWatcherManager(
	root context.Context,
	h *hub.Hub,
	workspace workspacerepo.Workspace,
	gitEngine enginegit.Engine,
	fsEngine enginefs.Engine,
	now func() time.Time,
) *WatcherManager {
	dispatcher := newWatcherDispatcher(h, workspace, now)
	provider := &gitStatusProvider{engine: gitEngine}
	factory := newWatcherFactory(fsEngine, provider, workspace, dispatcher)
	return newWatcherManager(root, factory, watcherLingerDuration, realTimer)
}

// newWatcherManager builds a WatcherManager over an injected factory, linger
// duration, and timer factory (the clock seam). root is the parent context for
// every watcher goroutine; cancelling it stops all watchers.
func newWatcherManager(
	root context.Context,
	factory watcherFactory,
	linger time.Duration,
	newTimer timerFactory,
) *WatcherManager {
	return &WatcherManager{
		root:     root,
		factory:  factory,
		linger:   linger,
		newTimer: newTimer,
		handles:  make(map[string]*watcherHandle),
	}
}

// Acquire records one subscriber for wsID. On the 0→1 transition it builds and
// starts the watcher in a goroutine under a cancelable context. A re-Acquire
// that lands while the watcher is lingering (post-release, pre-expiry) reuses
// the live watcher: it cancels the linger timer and bumps the handle generation
// so a stale timer callback that fires anyway is ignored — no teardown, no
// rebuild. A blank wsID is ignored so a misrouted subscribe can never start an
// unscoped watcher. After StopAll has run it is a no-op, so a late subscribe
// from a not-yet-closed hijacked WS connection cannot start a watcher under the
// cancelled root.
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
		if h.timer != nil {
			h.timer.Stop()
			h.timer = nil
			h.gen++
		}
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
		defer safego.Recover("realtime.watcher.start")
		_ = proc.Start(ctx)
	}()
}

// Release drops one subscriber for wsID. On the 1→0 transition it does NOT tear
// the watcher down immediately: it arms a linger timer so a reconnect within the
// grace period reuses the live watcher. Only when the timer fires (expire) is the
// watcher goroutine cancelled and stopped.
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
	h.gen++
	gen := h.gen
	h.timer = m.newTimer(m.linger, func() { m.expire(wsID, gen) })
	m.mu.Unlock()
}

// expire tears down a lingering watcher once its grace period elapses. The
// generation guard makes it a no-op if a reconnect reused the watcher, if a new
// release cycle re-armed the linger, or if StopAll already removed the handle —
// so a late-firing timer callback can never stop a watcher that has been revived.
func (m *WatcherManager) expire(
	wsID string,
	gen uint64,
) {
	m.mu.Lock()
	h, ok := m.handles[wsID]
	if !ok || h.gen != gen || h.refs > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.handles, wsID)
	m.mu.Unlock()

	h.cancel()
	h.proc.Stop()
}

// StopAll cancels and stops every live watcher — including any that is currently
// lingering — cancels their pending linger timers, and clears the handle map. It
// is idempotent and safe to call when no watcher is running: a manager holding
// nothing is a no-op. It runs on graceful shutdown so inotify file descriptors
// are closed promptly rather than waiting on process exit. Because it replaces
// the handle map, any linger timer that fires after StopAll finds no handle and
// no-ops.
func (m *WatcherManager) StopAll() {
	m.mu.Lock()
	m.closed = true
	handles := m.handles
	m.handles = make(map[string]*watcherHandle)
	m.mu.Unlock()

	for _, h := range handles {
		if h.timer != nil {
			h.timer.Stop()
		}
		h.cancel()
		h.proc.Stop()
	}
}
