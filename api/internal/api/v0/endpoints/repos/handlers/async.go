package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// runAsync runs fn in a detached goroutine after the handler has already
// written its 202, implementing the fail-fast/good-path-async pattern (00 §4):
// validation runs synchronously on the request path, then the slow work runs in
// the background and its outcome is delivered on the entity's WebSocket stream.
//
// The goroutine runs on context.WithoutCancel(parent) because the request ctx is
// cancelled the moment the 202 response is flushed. fn owns its own
// success/failure broadcasting; a failed repo mutation produces no DTO frame
// (there is no per-repo LastError sink), so the broadcast simply never fires.
//
// Every op is tracked on h.async so callers can block on its real completion;
// see WaitAsync.
func (h *Handlers) runAsync(
	parent context.Context,
	fn func(ctx context.Context),
) {
	ctx := context.WithoutCancel(parent)
	// Add runs on the request goroutine, BEFORE the spawn, so a WaitAsync that
	// happens-after the handler returned can never miss this op.
	h.async.Add(1)
	go func() {
		// Registered first, so it unwinds LAST — after the recovery boundary. A
		// panic in the detached op must not crash the daemon (safego.Recover logs
		// and contains it) and must still release the counter.
		defer h.async.Done()
		defer safego.Recover("repos.runAsync")
		fn(ctx)
	}()
}

// WaitAsync blocks until every detached runAsync op scheduled so far has fully
// returned — success, error, or panic.
//
// It is the real completion signal for the fire-and-forget handlers: a test that
// must assert a NEGATIVE ("the failed create broadcast no RepoDTO") can only do
// so soundly once the producing goroutine is provably dead, which a sleep never
// establishes. Because Add happens on the request goroutine before the spawn,
// WaitAsync also returns immediately — and correctly — when a fail-fast
// validation path scheduled no work at all.
func (h *Handlers) WaitAsync() { h.async.Wait() }
