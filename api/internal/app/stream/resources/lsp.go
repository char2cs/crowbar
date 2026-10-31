// Package resources holds the daemon work the stream runs while a client holds
// a scope naming it: file watchers, host-document watches, LSP hosts, forge
// polls and origin syncs. The stream's refcount decides when each starts and
// stops; each resource only does it.
package resources

import (
	"context"
	"errors"
	"sync"
)

// errClosed refuses a Start after Close.
var errClosed = errors.New("resources: closed")

// WorkspaceReleaser is the slice of the LSP engine the LSP resource needs.
type WorkspaceReleaser interface {
	ReleaseWorkspace(
		ctx context.Context,
		ownerID string,
	)
}

// LSP releases every language server of an owner when no client holds it any
// more. Servers start on document sync, so Start does no work; a client that
// drops without closing its documents would otherwise leak them.
type LSP struct {
	root   context.Context
	engine WorkspaceReleaser
	mu     sync.Mutex
	held   map[string]struct{}
	closed bool
}

// NewLSP builds the LSP resource over the engine; root bounds every release.
func NewLSP(
	root context.Context,
	engine WorkspaceReleaser,
) *LSP {
	return &LSP{root: root, engine: engine, held: map[string]struct{}{}}
}

// Start records that a client holds ownerID's session.
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

// Stop releases ownerID's language servers.
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

// Close releases every owner still held and refuses later starts.
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
