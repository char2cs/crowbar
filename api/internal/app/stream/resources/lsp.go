package resources

import (
	"context"
	"errors"
	"sync"
)

var errClosed = errors.New("resources: closed")

type WorkspaceReleaser interface {
	ReleaseWorkspace(
		ctx context.Context,
		ownerID string,
	)
}

type LSP struct {
	root   context.Context
	engine WorkspaceReleaser
	mu     sync.Mutex
	held   map[string]struct{}
	closed bool
}

func NewLSP(
	root context.Context,
	engine WorkspaceReleaser,
) *LSP {
	return &LSP{root: root, engine: engine, held: map[string]struct{}{}}
}

func (l *LSP) Start(
	ownerID string,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return errClosed
	}
	l.held[ownerID] = struct{}{}
	return nil
}

func (l *LSP) Stop(
	ownerID string,
) {
	l.mu.Lock()
	_, ok := l.held[ownerID]
	delete(l.held, ownerID)
	l.mu.Unlock()
	if ok {
		l.engine.ReleaseWorkspace(l.root, ownerID)
	}
}

func (l *LSP) Close() {
	l.mu.Lock()
	l.closed = true
	held := l.held
	l.held = map[string]struct{}{}
	l.mu.Unlock()
	for ownerID := range held {
		l.engine.ReleaseWorkspace(l.root, ownerID)
	}
}
