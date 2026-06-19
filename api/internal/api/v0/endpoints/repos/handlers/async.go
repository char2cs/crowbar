package handlers

import (
	"context"
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
func runAsync(
	parent context.Context,
	fn func(ctx context.Context),
) {
	ctx := context.WithoutCancel(parent)
	go fn(ctx)
}
