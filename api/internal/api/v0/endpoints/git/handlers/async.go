package handlers

import (
	"context"
	"fmt"

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
// The goroutine runs on context.WithoutCancel(parent) because the request ctx is
// cancelled the moment the 202 response is flushed.
func (h *Handlers) runAsync(
	parent context.Context,
	wsID string,
	fn func(ctx context.Context) error,
) {
	ctx := context.WithoutCancel(parent)
	go func() {
		// A panic in the detached git op must not crash the daemon; surface it on
		// the workspace entity (same channel as an error) instead of vanishing.
		defer safego.RecoverFn("git.runAsync", func(r any) {
			_, _ = h.lastErrors.SetLastError(ctx, wsID, fmt.Sprintf("internal error: %v", r))
		})
		if err := fn(ctx); err != nil {
			_, _ = h.lastErrors.SetLastError(ctx, wsID, err.Error())
		}
	}()
}
