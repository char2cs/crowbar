//go:build integration

package kit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

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

type EntityWatcher struct{ w *WSWatcher }

func (e *Env) WatchProjects(t *testing.T) *EntityWatcher {
	t.Helper()
	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/stream"))
	w.WriteJSON(t, stream.Request{Op: stream.OpSub, Scope: string(stream.KindProjects)})
	return &EntityWatcher{w: w}
}

func (e *Env) HoldProjects(t *testing.T) *EntityWatcher {
	t.Helper()
	return &EntityWatcher{w: e.HoldScope(t, string(stream.KindProjects))}
}

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
