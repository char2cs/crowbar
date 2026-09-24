package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_ReattachOversizedPartialBoundedEmpty pins the §8.3 bound: an incomplete
// sequence longer than maxPendingPartial is dropped, so PendingInput() is empty and the Attach
// payload carries no dangling tail — the unbounded-buffer footgun cannot leak a giant partial
// into the redraw.
func TestRegression_ReattachOversizedPartialBoundedEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-midsplit-big", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	// An OSC title-set opener with no ST/BEL terminator and a body far past maxPendingPartial.
	huge := strings.Repeat("a", 8192)
	s.PumpChunkForTest(append([]byte("\x1b]0;"), []byte(huge)...))

	require.Empty(t, s.model.PendingInput(),
		"an over-long incomplete sequence must be dropped, not buffered")

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver a redraw frame")
	assert.NotContains(t, string(f.Data), huge,
		"the dropped over-long partial must not be appended to the redraw payload")
}

// lastBytes returns up to n trailing bytes of b, for readable failure messages.
func lastBytes(
	b []byte,
	n int,
) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}
