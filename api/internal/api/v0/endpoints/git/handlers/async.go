package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// runAsync runs the slow git op fn in a detached goroutine after the handler has
// already written its 202, implementing the fail-fast/good-path-async pattern
// (00 §4): validation runs synchronously on the request path, then the slow git
// work runs in the background. On success the existing git-status watcher
// broadcasts the post-op state; on a non-nil fn error the failure is surfaced on
// the workspace entity via SetLastError(wsID, message) — errors live on the
// entity, never on a separate WS frame.
//
// The op outlives the request ctx, which is cancelled the moment the 202 is
// flushed; a daemon shutdown waits for it (Shutdown).
func (h *Handlers) runAsync(
	parent context.Context,
	wsID string,
	fn func(ctx context.Context) error,
) {
	h.working.BeginWork(context.WithoutCancel(parent), wsID)
	h.async.Go(parent, "git.runAsync", func(ctx context.Context) {
		// A panic in the detached git op must not crash the daemon; release the
		// working overlay, then surface it on the workspace entity (same channel
		// as an error) instead of vanishing.
		defer safego.RecoverFn("git.runAsync", func(r any) {
			h.working.EndWork(ctx, wsID)
			h.recordLastError(ctx, wsID, fmt.Sprintf("internal error: %v", r))
		})
		err := fn(ctx)
		h.working.EndWork(ctx, wsID)
		if err != nil {
			h.recordLastError(ctx, wsID, err.Error())
		}
	})
}

// Shutdown waits for the detached ops, cancelling them if ctx ends first.
func (h *Handlers) Shutdown(ctx context.Context) error { return h.async.Shutdown(ctx) }

// recordLastError surfaces a failed op on the workspace entity. It is the
// failure sink itself, so its own failure is logged: dropping it would leave
// an op that failed with no trace at all.
func (h *Handlers) recordLastError(
	ctx context.Context,
	wsID string,
	message string,
) {
	if _, err := h.lastErrors.SetLastError(ctx, wsID, message); err != nil {
		slog.ErrorContext(ctx, "git: record the failed op on the workspace", "ws", wsID, "op_err", message, "err", err)
	}
}
