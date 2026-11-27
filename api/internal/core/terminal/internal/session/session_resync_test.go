//go:build !windows

package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func drainFrames(ch <-chan OutputFrame) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestAttach_FirstFrameIsSnapshot(t *testing.T) {
	s, err := New(t.Context(), "sid-snap-attach", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 0)
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
