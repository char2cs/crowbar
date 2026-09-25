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
// The op outlives the request ctx, which is cancelled the moment the 202 is
// flushed; a daemon shutdown waits for it (Shutdown). On a non-nil fn error the
// failure is surfaced on the workspace entity via broadcastOnErr(wsID, message)
// — errors live on the entity, never on a separate WS frame.
func (h *Handlers) runAsync(
	parent context.Context,
	work WorkSignal,
	broadcastOnErr func(ctx context.Context, wsID, message string),
	wsID string,
	fn func(ctx context.Context) error,
) {
	work.BeginWork(context.WithoutCancel(parent), wsID)
	h.async.Go(parent, "worktree.runAsync", func(ctx context.Context) {
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
	})
}

// WaitAsync blocks until every detached runAsync op scheduled so far has fully
// returned — success, error, or panic — so every observable effect of the op
// has already happened. Because the op is counted on the request goroutine
// before the spawn, it also returns at once when a fail-fast validation path
// scheduled no work at all.
func (h *Handlers) WaitAsync() { h.async.Wait() }

// Shutdown waits for the detached ops, cancelling them if ctx ends first.
func (h *Handlers) Shutdown(ctx context.Context) error { return h.async.Shutdown(ctx) }

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
