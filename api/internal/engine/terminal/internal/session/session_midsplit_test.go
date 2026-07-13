package session

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// TestRegression_ReattachMidSplitSequence pins the §8.3 mid-sequence re-sync on the
// DEGRADED/raw path (the ONLY path that still appends the partial, per I3): when a
// live chunk ends mid-escape-sequence and the session is streaming raw bytes, Attach
// appends model.PendingInput() AFTER the clean redraw so a freshly attached client's
// parser converges to the same in-flight boundary the live clients hold — the raw
// continuation genuinely follows over the byte stream. It asserts (a) the Attach
// payload ends with the buffered partial, (b) the live tail then completes correctly
// against that payload (the move-and-overwrite happens, the tail is NOT rendered as
// literal text), and (c) Snapshot()/.buf EXCLUDE the partial (a clean, fully-
// terminated persisted frame). Deleting the raw-path PendingInput append regresses
// (a)/(b). (The HEALTHY model-driven path must NOT append — the continuation never
// arrives there — which TestModelDriven_HealthyAttachSnapshotHasNoPendingInput pins.)
func TestRegression_ReattachMidSplitSequence(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-midsplit", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	// Feed a chunk that prints "HELLO" then ENDS mid-sequence with an incomplete CSI "ESC[5".
	// The completion "DX" is CSI 5 D (cursor back 5) followed by 'X': interpreted it walks the
	// cursor back over "HELLO" and overwrites 'H' → "XELLO"; mis-parsed as literal text it
	// prints "5DX" verbatim → "...5DX". Fed via the production pumpStep (no client yet, so no
	// fan-out), leaving the model mid-sequence with pendingPartial == "ESC[5".
	s.PumpChunkForTest([]byte("HELLO\x1b[5"))

	pending := append([]byte(nil), s.model.PendingInput()...)
	require.Equal(t, "\x1b[5", string(pending), "model must hold the incomplete CSI as pending input")

	// Force the raw/degraded fallback so the Attach appends the partial: under
	// healthy model-driven emission the client would never receive the raw
	// continuation, so the append is now gated on !modelEmitHealthyLocked() (I3).
	forceModelPanicForTest(s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver a redraw frame")
	payload := f.Data

	// (a) The payload ends with exactly the buffered mid-sequence partial.
	require.True(t, bytes.HasSuffix(payload, pending),
		"attach payload must end with the buffered partial %q; got tail %q",
		pending, lastBytes(payload, 8))

	// (b) Replaying the payload + the live tail into a fresh same-size client parser reproduces
	// the interpreted result ("XELLO"), NOT the literal mis-render ("5DX").
	tail := []byte("DX")
	vm, vser := model.New(80, 24, 1000)
	vm.Write(payload)
	vm.Write(tail)
	rendered := string(vser.Serialize(vm))
	assert.Contains(t, rendered, "XELLO",
		"payload+tail must render the interpreted move-and-overwrite result")
	assert.NotContains(t, rendered, "5DX",
		"payload+tail must NOT render the tail as literal text")

	// (b′) Prove the PendingInput append is load-bearing: the SAME payload with the partial
	// stripped (what a redraw-only attach would send, leaving the client parser in ground
	// state) mis-renders — the tail "DX" lands as literal text appended after "HELLO" instead
	// of completing the CUB-5 that overwrites the 'H'.
	naive := payload[:len(payload)-len(pending)]
	nm, nser := model.New(80, 24, 1000)
	nm.Write(naive)
	nm.Write(tail)
	naiveRender := string(nser.Serialize(nm))
	assert.Contains(t, naiveRender, "HELLODX",
		"a redraw-only payload (no pending append) must append the tail as literal text")
	assert.NotContains(t, naiveRender, "XELLO",
		"a redraw-only payload must NOT reproduce the interpreted overwrite — this is the "+
			"regression the Attach PendingInput append prevents")

	// (c) Snapshot/.buf is a clean, fully-terminated frame that EXCLUDES the mid-sequence
	// partial — the persisted blob never carries a dangling escape tail.
	blob, _ := s.Snapshot()
	assert.False(t, bytes.HasSuffix(blob, pending),
		"Snapshot blob must NOT end with the mid-sequence partial; got tail %q", lastBytes(blob, 8))
}

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
