package realtime

import "context"

// LSPLifecycle is the per-workspace LSP host lifecycle the LSPManager drives on
// its refcount edges. Ensure runs on the first LSP subscriber for a workspace;
// Shutdown runs on the last.
//
// Nuance (10 §1, lsp.Engine): the real LSP engine is document-driven — language
// servers are spawned lazily on textDocument/didOpen and kept warm by the
// open-document refcount. There is no per-workspace "spawn every server" call,
// so Ensure does no work. The teardown edge, however, MUST do work: the only
// per-document decrement-to-zero path is an explicit DidClose, so a force-quit,
// crash, or WS-drop that loses the connection without sending DidClose for every
// open file would otherwise leak the workspace's language servers forever. The
// production lifecycle therefore releases the whole workspace on the last
// LSP-subscriber edge (R11), mirroring how the file watcher stops on its own
// last-unsubscribe edge.
type LSPLifecycle interface {
	Ensure(
		ctx context.Context,
		wsID string,
	)
	Shutdown(
		ctx context.Context,
		wsID string,
	)
}

// WorkspaceReleaser is the slice of lsp.Engine the production lifecycle needs:
// the ability to tear down every server for a workspace regardless of refcount.
type WorkspaceReleaser interface {
	ReleaseWorkspace(
		ctx context.Context,
		wsID string,
	)
}

// NewLSPLifecycle returns the production LSP lifecycle over the LSP engine.
// Ensure is a no-op (servers follow document sync, not subscription edges);
// Shutdown releases all of the workspace's language servers on the last
// LSP-subscriber edge so a WS-drop with files still open does not leak them.
func NewLSPLifecycle(
	engine WorkspaceReleaser,
) LSPLifecycle {
	return engineLSPLifecycle{engine: engine}
}

type engineLSPLifecycle struct {
	engine WorkspaceReleaser
}

func (engineLSPLifecycle) Ensure(
	_ context.Context,
	_ string,
) {
}

func (l engineLSPLifecycle) Shutdown(
	ctx context.Context,
	wsID string,
) {
	l.engine.ReleaseWorkspace(ctx, wsID)
}

// NoopLSPLifecycle returns a lifecycle whose edges do no work. It is used in
// tests and any wiring that has no LSP engine to release; production uses
// NewLSPLifecycle so the last-unsubscribe edge releases the workspace's servers.
func NoopLSPLifecycle() LSPLifecycle {
	return noopLSPLifecycle{}
}

type noopLSPLifecycle struct{}

func (noopLSPLifecycle) Ensure(
	_ context.Context,
	_ string,
) {
}

func (noopLSPLifecycle) Shutdown(
	_ context.Context,
	_ string,
) {
}
