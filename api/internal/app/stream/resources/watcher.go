package resources

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// watcherLingerDuration is the grace period a watcher stays alive after its
// last holder leaves: a reconnect within it reuses the live watcher instead of
// rebuilding it (a recursive walk of the whole worktree), yet a closed
// workspace still releases its inotify FDs promptly.
const watcherLingerDuration = 10 * time.Second

// watcherProc is the lifecycle surface the Watcher drives. *watch.Watcher
// satisfies it; tests inject a fake to assert lazy start/stop without fsnotify.
type watcherProc interface {
	// Start begins watching under ctx and returns once the watch runs. ctx bounds
	// the whole watch, so cancelling it also cuts a slow start short.
	Start(
		ctx context.Context,
	) error
	// Stop ends a running watch and waits for it (a Start in flight is the Watcher's to join).
	Stop()
}

// watcherFactory builds the unstarted watcherProc of a workspace.
type watcherFactory func(
	ctx context.Context,
	wsID string,
) (watcherProc, error)

// lingerTimer is the one-shot expiry timer armed by Stop; *time.Timer satisfies it.
type lingerTimer interface {
	Stop() bool
}

// timerFactory builds a lingerTimer that invokes fn after d: the clock seam,
// so tests drive linger expiry explicitly, never by wall-clock waits.
type timerFactory func(
	d time.Duration,
	fn func(),
) lingerTimer

func realTimer(
	d time.Duration,
	fn func(),
) lingerTimer {
	return time.AfterFunc(d, fn)
}

// watcherHandle is one workspace's running watcher: the cancel that stops its
// Start goroutine, its linger timer (set only while lingering after a Stop),
// and a generation that voids a linger timer a later Start or Stop superseded.
type watcherHandle struct {
	cancel context.CancelFunc
	proc   watcherProc
	timer  lingerTimer
	gen    uint64
}

// Watcher runs a workspace's file watcher while a client holds its git: or
// files: scope: the watcher is what produces git status, file changes and the
// SyncWorkingTreeState command.
type Watcher struct {
	root     context.Context
	factory  watcherFactory
	linger   time.Duration
	newTimer timerFactory
	mu       sync.Mutex
	handles  map[string]*watcherHandle
	closed   bool
	// live counts Start goroutines and expiries still stopping a watcher;
	// Close joins them so no watcher outlives it.
	live sync.WaitGroup
}

// NewWatcher builds the production Watcher: the dispatcher signals the hub and
// syncs the workspace aggregate, the git status provider adapts the git engine,
// and the factory resolves worktree paths. root parents every watcher goroutine.
func NewWatcher(
	root context.Context,
	signals Signaler,
	workspace workspacerepo.Workspace,
	gitEngine enginegit.Engine,
	fsEngine enginefs.Engine,
	now func() time.Time,
) *Watcher {
	dispatcher := newWatcherDispatcher(signals, workspace, now)
	provider := &gitStatusProvider{engine: gitEngine}
	factory := newWatcherFactory(fsEngine, provider, workspace, gitEngine, dispatcher)
	return newWatcher(root, factory, watcherLingerDuration, realTimer)
}

func newWatcher(
	root context.Context,
	factory watcherFactory,
	linger time.Duration,
	newTimer timerFactory,
) *Watcher {
	return &Watcher{
		root:     root,
		factory:  factory,
		linger:   linger,
		newTimer: newTimer,
		handles:  make(map[string]*watcherHandle),
	}
}

// Start builds and starts wsID's watcher in a goroutine, or revives the one
// still lingering after a Stop. A workspace that cannot be watched is refused.
func (m *Watcher) Start(
	wsID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errClosed
	}
	if h, ok := m.handles[wsID]; ok {
		if h.timer != nil {
			h.timer.Stop()
			h.timer = nil
			h.gen++
		}
		return nil
	}
	proc, err := m.factory(m.root, wsID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(m.root)
	m.handles[wsID] = &watcherHandle{cancel: cancel, proc: proc}
	m.live.Add(1)
	go func() {
		defer m.live.Done()
		defer safego.Recover("resources.watcher.start")
		err := proc.Start(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			// Only expire and Close cancel, and both already removed the handle.
			return
		}
		slog.WarnContext(ctx, "resources: watcher start failed", "workspace_id", wsID, "err", err)
		m.retire(wsID, proc)
	}()
	return nil
}

// retire drops a handle whose watcher died in Start, so the next Start
// rebuilds it. It spares a successor an expire and restart already put there.
func (m *Watcher) retire(
	wsID string,
	proc watcherProc,
) {
	m.mu.Lock()
	h, ok := m.handles[wsID]
	if !ok || h.proc != proc {
		m.mu.Unlock()
		return
	}
	delete(m.handles, wsID)
	if h.timer != nil {
		h.timer.Stop()
	}
	m.mu.Unlock()

	h.cancel()
	h.proc.Stop()
}

// Stop arms the linger timer instead of tearing the watcher down: a Start
// within the grace period reuses it, and only expire stops it.
func (m *Watcher) Stop(
	wsID string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.handles[wsID]
	if !ok {
		return
	}
	h.gen++
	gen := h.gen
	h.timer = m.newTimer(m.linger, func() { m.expire(wsID, gen) })
}

// expire tears down a lingering watcher once its grace period elapses. The
// generation guard makes a superseded or late timer a no-op, so it never
// stops a watcher that was revived.
func (m *Watcher) expire(
	wsID string,
	gen uint64,
) {
	m.mu.Lock()
	h, ok := m.handles[wsID]
	if !ok || h.gen != gen || h.timer == nil {
		m.mu.Unlock()
		return
	}
	delete(m.handles, wsID)
	m.live.Add(1)
	m.mu.Unlock()
	defer m.live.Done()

	h.cancel()
	h.proc.Stop()
}

// Close stops every watcher, lingering ones included, then joins every Start
// and expiry still running. It is idempotent and refuses later starts.
func (m *Watcher) Close() {
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
	m.live.Wait()
}
