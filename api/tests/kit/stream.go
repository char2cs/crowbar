//go:build integration

package kit

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// HoldScope opens a /v0/stream connection holding scope and returns once its
// snapshot is delivered, so every frame read after it is live.
func (e *Env) HoldScope(
	t *testing.T,
	scope string,
) *WSWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/stream"))
	w.WriteJSON(t, stream.Request{Op: stream.OpSub, Scope: scope})
	w.ReadUntil(t, 10*time.Second, func(frame map[string]any) bool {
		return frame["t"] == stream.FrameSynced && frame["scope"] == scope
	})
	return w
}
