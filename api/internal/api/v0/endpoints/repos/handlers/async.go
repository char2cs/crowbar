package handlers

import (
	"context"
)

// runAsync runs fn in a detached goroutine after the handler has already
// written its 202, implementing the fail-fast/good-path-async pattern (00 §4):
// validation runs synchronously on the request path, then the slow work runs in
// the background and its outcome is delivered on the entity's WebSocket stream.
//
// The op outlives the request ctx, which is cancelled the moment the 202 is
// flushed; a daemon shutdown waits for it (Shutdown). fn owns its own
// success/failure broadcasting; a failed repo mutation produces no DTO frame
// (there is no per-repo LastError sink), so the broadcast simply never fires.
func (h *Handlers) runAsync(
	parent context.Context,
	fn func(ctx context.Context),
) {
	h.async.Go(parent, "repos.runAsync", fn)
}

// WaitAsync blocks until every detached runAsync op scheduled so far has fully
// returned — success, error, or panic. Because the op is counted on the request
// goroutine before the spawn, it also returns at once when a fail-fast
// validation path scheduled no work at all.
func (h *Handlers) WaitAsync() { h.async.Wait() }

// Shutdown waits for the detached ops, cancelling them if ctx ends first.
func (h *Handlers) Shutdown(ctx context.Context) error { return h.async.Shutdown(ctx) }
