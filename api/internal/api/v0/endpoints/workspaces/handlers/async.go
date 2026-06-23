package handlers

import (
	"context"
)

// runAsync runs fn in a detached goroutine after the handler has already
// written its 202, implementing the fail-fast/good-path-async pattern (00 §4):
// validation runs synchronously on the request path, then the slow work runs in
// the background and its outcome is delivered on the entity's WebSocket stream.
//
// The goroutine runs on context.WithoutCancel(parent) because the request ctx
// is cancelled the moment the 202 response is flushed. On a non-nil fn error the
// failure is surfaced on the workspace entity via broadcastOnErr(wsID, message)
// — errors live on the entity, never on a separate WS frame.
func runAsync(
	parent context.Context,
	broadcastOnErr func(ctx context.Context, wsID string, message string),
	wsID string,
	fn func(ctx context.Context) error,
) {
	ctx := context.WithoutCancel(parent)
	go func() {
		if err := fn(ctx); err != nil {
			broadcastOnErr(ctx, wsID, err.Error())
		}
	}()
}
