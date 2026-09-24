//go:build !windows

package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainFrames pulls everything currently queued on ch without blocking.
func drainFrames(ch <-chan OutputFrame) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// waitSnapshotFrame blocks until a Snapshot frame arrives on ch, skipping any incremental
// diff frames on the way. A closed channel (the session died) is the only way it reports
// false — there is no timeout: a snapshot that never arrives is a hang, and `go test -timeout`
// reports it with the blocked stack rather than a bespoke "not within 3s" message.
func waitSnapshotFrame(ch <-chan OutputFrame) bool {
	for f := range ch {
		if f.Snapshot {
			return true
		}
	}
	return false
}

// TestAttach_FirstFrameIsSnapshot pins the attach redraw's Snapshot marking:
// the client applies it onto a reset buffer, so it must be distinguishable
// from incremental output.
func TestAttach_FirstFrameIsSnapshot(t *testing.T) {
	s, err := New("sid-snap-attach", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver a redraw frame")
	assert.True(t, f.Snapshot, "the attach redraw frame must be marked Snapshot")
	assert.NotEmpty(t, f.Data)
}
