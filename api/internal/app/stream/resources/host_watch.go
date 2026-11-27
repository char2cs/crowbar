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

type Signaler interface {
	Signal(scope stream.Scope, kind string, data any)
}

type fileWatcherFactory func(
	path string,
	emit func(change domain.FileChangeType),
) watcherProc

type hostWatch struct {
	proc watcherProc
}

type HostWatch struct {
	root      context.Context
	factory   fileWatcherFactory
	broadcast func(domain.FileChangeEvent)

	mu      sync.Mutex
	watches map[string]*hostWatch
	closed  bool
}

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
