package handlers

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// runAsync runs fn in a detached goroutine after the handler has already
// written its 202, implementing the fail-fast/good-path-async pattern (00 §4):
// validation runs synchronously on the request path, then the slow work runs in
// the background and its outcome is delivered on the entity's WebSocket stream.
//
// The op is bracketed with work.BeginWork/EndWork so the entity's derived
// Working overlay tracks real daemon activity: BeginWork fires synchronously
// (the spinner starts with the 202, racing nothing) and EndWork fires on every
// exit — success, error, or panic — before the failure is surfaced, so the
// final frame a client sees always carries Working=false.
//
// The goroutine runs on context.WithoutCancel(parent) because the request ctx is
// cancelled the moment the 202 response is flushed. On a non-nil fn error the
// failure is surfaced on the workspace entity via broadcastOnErr(wsID, message)
// — errors live on the entity, never on a separate WS frame.
//
// Every op is tracked on h.async so callers can block on real completion; see
// WaitAsync.
func (h *Handlers) runAsync(
	parent context.Context,
	work WorkSignal,
	broadcastOnErr func(ctx context.Context, wsID, message string),
	wsID string,
	fn func(ctx context.Context) error,
) {
	ctx := context.WithoutCancel(parent)
	work.BeginWork(ctx, wsID)
	// Add runs on the request goroutine, BEFORE the spawn, so a WaitAsync that
	// happens-after the handler returned can never miss this op.
	h.async.Add(1)
	go func() {
		// Registered first, so it unwinds LAST — after the panic path below has
		// released the overlay and surfaced the failure. WaitAsync therefore only
		// returns once every observable effect of the op has already happened.
		defer h.async.Done()
		// A panic in the detached op must not crash the daemon; release the
		// working overlay, then surface it on the workspace entity (the same
		// channel as an error) instead of letting it vanish.
		defer safego.RecoverFn("worktree.runAsync", func(r any) {
			work.EndWork(ctx, wsID)
			broadcastOnErr(ctx, wsID, fmt.Sprintf("internal error: %v", r))
		})
		err := fn(ctx)
		work.EndWork(ctx, wsID)
		if err != nil {
			broadcastOnErr(ctx, wsID, err.Error())
		}
	}()
}

// WaitAsync blocks until every detached runAsync op scheduled so far has fully
// returned — success, error, or panic (Done is deferred first, so it releases
// after the recovery handler has run).
//
// It is the real completion signal for the fire-and-forget handlers: a test that
// must assert a NEGATIVE ("the failed op broadcast nothing") can only do so
// soundly once the producing goroutine is provably dead, which a sleep never
// establishes. Because Add happens on the request goroutine before the spawn,
// WaitAsync also returns immediately — and correctly — when a fail-fast
// validation path scheduled no work at all.
func (h *Handlers) WaitAsync() { h.async.Wait() }

// broadcastLastError records a failed background mutation on the workspace
// entity, which is the one channel a detached op has to report through. A blank
// wsID (the batch import, which has no entity until it produces one) is a no-op
// since there is nothing to attach the error to.
func (h *Handlers) broadcastLastError(
	ctx context.Context,
	wsID string,
	message string,
) {
	if wsID == "" {
		return
	}
	_, _ = h.lastErrors.SetLastError(ctx, wsID, message)
}
