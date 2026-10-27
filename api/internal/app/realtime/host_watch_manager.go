package realtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// errHostWatchesClosed refuses an Acquire after StopAll.
var errHostWatchesClosed = errors.New("realtime: host watches are closed")

// fileWatcherFactory builds the unstarted watch on one held path; emit runs on
// the watch's own goroutine for each change to the file.
type fileWatcherFactory func(
	path string,
	emit func(change domain.FileChangeType),
) watcherProc

// hostWatch is one held path's watch. armed closes once the first holder's
// Start returned, with err its answer.
type hostWatch struct {
	holders int
	proc    watcherProc
	armed   chan struct{}
	err     error
}

// HostWatchManager refcounts the watches on host documents clients hold open:
// one watch per held path, armed by its first holder and stopped by its last.
type HostWatchManager struct {
	root      context.Context
	factory   fileWatcherFactory
	broadcast func(domain.FileChangeEvent)

	mu      sync.Mutex
	watches map[string]*hostWatch
	closed  bool
}

// NewHostWatchManager builds the manager over the fs engine's file watchers,
// broadcasting each change through the hub.
func NewHostWatchManager(
	root context.Context,
	h *hub.Hub,
	fsEngine enginefs.Engine,
) *HostWatchManager {
	factory := func(path string, emit func(domain.FileChangeType)) watcherProc {
		return fsEngine.NewFileWatcher(path, emit)
	}
	return newHostWatchManager(root, factory, h.BroadcastFile)
}

func newHostWatchManager(
	root context.Context,
	factory fileWatcherFactory,
	broadcast func(domain.FileChangeEvent),
) *HostWatchManager {
	return &HostWatchManager{
		root:      root,
		factory:   factory,
		broadcast: broadcast,
		watches:   make(map[string]*hostWatch),
	}
}

// Acquire holds the canonical path for one client and returns once its watch is
// armed, or why it could not be. The first holder arms outside the manager
// lock, so a slow mount holds back only the clients of its own path.
func (m *HostWatchManager) Acquire(
	path string,
) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errHostWatchesClosed
	}
	if hw, ok := m.watches[path]; ok {
		hw.holders++
		m.mu.Unlock()
		<-hw.armed
		return hw.err
	}
	hw := &hostWatch{holders: 1, armed: make(chan struct{})}
	hw.proc = m.factory(path, m.emitter(path, hw))
	m.watches[path] = hw
	m.mu.Unlock()

	if err := hw.proc.Start(m.root); err != nil {
		hw.err = fmt.Errorf("realtime: watch host document: %w", err)
		slog.Warn("realtime: watch host document", "path", path, "err", err)
		m.mu.Lock()
		if m.watches[path] == hw {
			delete(m.watches, path)
		}
		m.mu.Unlock()
	}
	close(hw.armed)
	return hw.err
}

// Release drops one client's hold; the last one stops the watch, after the
// lock is released, since the watch's emit takes it.
func (m *HostWatchManager) Release(
	path string,
) {
	m.mu.Lock()
	hw, ok := m.watches[path]
	if !ok {
		m.mu.Unlock()
		return
	}
	hw.holders--
	if hw.holders > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.watches, path)
	m.mu.Unlock()
	hw.proc.Stop()
}

// StopAll stops every watch and refuses every later Acquire.
func (m *HostWatchManager) StopAll() {
	m.mu.Lock()
	m.closed = true
	watches := m.watches
	m.watches = make(map[string]*hostWatch)
	m.mu.Unlock()
	for _, hw := range watches {
		hw.proc.Stop()
	}
}

// emitter broadcasts the held spelling of path, never the one the OS reports,
// and drops the changes of a watch that is no longer path's current one.
func (m *HostWatchManager) emitter(
	path string,
	hw *hostWatch,
) func(domain.FileChangeType) {
	return func(change domain.FileChangeType) {
		m.mu.Lock()
		current := m.watches[path] == hw
		m.mu.Unlock()
		if current {
			m.broadcast(domain.FileChangeEvent{Type: change, Authority: domain.DocumentAuthorityHost, Path: path})
		}
	}
}
