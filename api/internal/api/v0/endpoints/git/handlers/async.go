package handlers

import (
	"context"
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
		if err := fn(ctx); err != nil {
			_, _ = h.lastErrors.SetLastError(ctx, wsID, err.Error())
		}
	}()
}
