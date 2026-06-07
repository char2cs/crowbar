package realtime

import "context"

// LSPLifecycle is the per-workspace LSP host lifecycle the LSPManager drives on
// its refcount edges. Ensure runs on the first LSP subscriber for a workspace;
// Shutdown runs on the last.
//
// Nuance (10 §1, lsp.Engine): the real LSP engine is document-driven — language
// servers are spawned lazily on textDocument/didOpen and torn down when the last
// open document for a (wsID, languageID) is released, all via the synchronous
// editor-feature path. There is no per-workspace "spawn every server" or
// "shut down the whole workspace" call on lsp.Engine. The production lifecycle
// is therefore the noop lifecycle: the LSPManager still owns the correct
// independent refcount so the topology and the Files∪Git vs LSP separation hold,
// but the actual server processes follow document sync, not subscription edges.
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

// NoopLSPLifecycle returns the production LSP lifecycle: the LSP engine has no
// subscription-tied spawn/shutdown, so the refcount edges do no work.
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
