//go:build integration

package kit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

// EntityWatcher reads one /v0/stream connection's entity frames.
type EntityWatcher struct{ w *WSWatcher }

// WatchProjects holds the projects scope WITHOUT skipping its snapshot, so a
// row that existed before the dial is read as well as every live one after it.
func (e *Env) WatchProjects(t *testing.T) *EntityWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/stream"))
	w.WriteJSON(t, stream.Request{Op: stream.OpSub, Scope: string(stream.KindProjects)})
	return &EntityWatcher{w: w}
}

// HoldProjects holds the projects scope past its snapshot, so every row read
// from it is a live change.
func (e *Env) HoldProjects(t *testing.T) *EntityWatcher {
	t.Helper()
	return &EntityWatcher{w: e.HoldScope(t, string(stream.KindProjects))}
}

// entityData is one entity frame's row; a gone frame answers its id and gone=true.
func entityData(t *testing.T, frame map[string]any, kind stream.EntityKind) (map[string]any, bool) {
	t.Helper()
	if frame["t"] != stream.FrameEntity || frame["kind"] != string(kind) {
		return nil, false
	}
	if gone, _ := frame["gone"].(bool); gone {
		return map[string]any{"id": frame["id"], "gone": true}, true
	}
	data, ok := frame["data"].(map[string]any)
	require.True(t, ok, "kit: an entity frame carries its row: %v", frame)
	return data, true
}

// Next answers the first row of kind that satisfies match within timeout.
func (w *EntityWatcher) Next(
	t *testing.T,
	timeout time.Duration,
	kind stream.EntityKind,
	match func(row map[string]any) bool,
) map[string]any {
	t.Helper()
	var got map[string]any
	w.w.ReadUntil(t, timeout, func(frame map[string]any) bool {
		row, ok := entityData(t, frame, kind)
		if ok && match(row) {
			got = row
			return true
		}
		return false
	})
	return got
}
