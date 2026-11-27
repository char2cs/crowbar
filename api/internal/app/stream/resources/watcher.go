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

const watcherLingerDuration = 10 * time.Second

type watcherProc interface {
	Start(
		ctx context.Context,
	) error
	Stop()
}

type watcherFactory func(
	ctx context.Context,
	wsID string,
) (watcherProc, error)

type lingerTimer interface {
	Stop() bool
}

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

type watcherHandle struct {
	cancel context.CancelFunc
	proc   watcherProc
	timer  lingerTimer
	gen    uint64
}

type Watcher struct {
	root     context.Context
	factory  watcherFactory
	linger   time.Duration
	newTimer timerFactory
	mu       sync.Mutex
	handles  map[string]*watcherHandle
	closed   bool
	live     sync.WaitGroup
}

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
			return
		}
		slog.WarnContext(ctx, "resources: watcher start failed", "workspace_id", wsID, "err", err)
		m.retire(wsID, proc)
	}()
	return nil
}

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
