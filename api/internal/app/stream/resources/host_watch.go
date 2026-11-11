package resources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// Signaler is where a resource's news goes: the stream hub.
type Signaler interface {
	Signal(scope stream.Scope, kind string, data any)
}

// fileWatcherFactory builds the unstarted watch on one held path; emit runs on
// the watch's own goroutine for each change to the file.
type fileWatcherFactory func(
	path string,
	emit func(change domain.FileChangeType),
) watcherProc

// hostWatch is one held path's watch; its identity tells a stale one apart.
type hostWatch struct {
	proc watcherProc
}

// HostWatch watches the host documents clients hold open, one watch per path,
// armed before Start returns so a read made after it misses no change.
type HostWatch struct {
	root      context.Context
	factory   fileWatcherFactory
	broadcast func(domain.FileChangeEvent)

	mu      sync.Mutex
	watches map[string]*hostWatch
	closed  bool
}

// NewHostWatch builds the resource over the fs engine's file watchers,
// signalling each change on the path's doc scope.
func NewHostWatch(
	root context.Context,
	signals Signaler,
	fsEngine enginefs.Engine,
) *HostWatch {
	factory := func(path string, emit func(domain.FileChangeType)) watcherProc {
		return fsEngine.NewFileWatcher(path, emit)
	}
	return newHostWatch(root, factory, func(e domain.FileChangeEvent) {
		signals.Signal(stream.Scope{Kind: stream.KindDoc, Key: e.Path}, stream.SignalFileChange, e)
	})
}

func newHostWatch(
	root context.Context,
	factory fileWatcherFactory,
	broadcast func(domain.FileChangeEvent),
) *HostWatch {
	return &HostWatch{
		root:      root,
		factory:   factory,
		broadcast: broadcast,
		watches:   make(map[string]*hostWatch),
	}
}

// Start arms path's watch outside the lock, so a slow mount holds back only
// the clients of its own path. A path the OS has no watches left for is
// refused with a code of its own: the client words it apart.
func (m *HostWatch) Start(
	path string,
) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errClosed
	}
	hw := &hostWatch{}
	hw.proc = m.factory(path, m.emitter(path, hw))
	m.watches[path] = hw
	m.mu.Unlock()

	err := hw.proc.Start(m.root)
	if err == nil {
		return nil
	}
	slog.Warn("resources: watch host document", "path", path, "err", err)
	m.mu.Lock()
	if m.watches[path] == hw {
		delete(m.watches, path)
	}
	m.mu.Unlock()
	if errors.Is(err, enginefs.ErrWatchLimit) {
		return &problem.Error{Code: problem.WatchLimit, Message: "resources: watch host document", Err: err}
	}
	return fmt.Errorf("resources: watch host document: %w", err)
}

// Stop ends path's watch after the lock is released, since its emit takes it.
func (m *HostWatch) Stop(
	path string,
) {
	m.mu.Lock()
	hw, ok := m.watches[path]
	delete(m.watches, path)
	m.mu.Unlock()
	if ok {
		hw.proc.Stop()
	}
}

// Close stops every watch and refuses every later Start.
func (m *HostWatch) Close() {
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
func (m *HostWatch) emitter(
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
