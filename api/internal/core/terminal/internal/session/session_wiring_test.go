package session

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The old `settle` helper lived here: it sampled SerializedLen every 50ms and called the shell
// "quiet" once the length had not changed across 5 samples. That is inference from silence — a
// guess that no straggler chunk will land after the window — and it is precisely the guess that
// loses on a loaded machine. Every caller now uses waitPrompt (session_test.go), which blocks on
// the shell's own statement that it has finished everything: its prompt.

func TestParseBlob(t *testing.T) {
	cols, rows, sb, body := parseBlob([]byte("CRWB1 100 40 1 5000\nhello world"))
	assert.Equal(t, 100, cols)
	assert.Equal(t, 40, rows)
	assert.Equal(t, 5000, sb)
	assert.Equal(t, []byte("hello world"), body)

	// No newline → not a header at all.
	c, r, s, b := parseBlob([]byte("no newline anywhere"))
	assert.Zero(t, c+r+s)
	assert.Nil(t, b)

	// A stale raw (non-CRWB1) blob with a newline → rejected, treated as empty.
	c, r, s, b = parseBlob([]byte("garbage line\nmore bytes"))
	assert.Zero(t, c+r+s)
	assert.Nil(t, b)
}

func TestResolveBirthDefaults(t *testing.T) {
	assert.Equal(t, 80, resolveCols(0))
	assert.Equal(t, 120, resolveCols(120))
	assert.Equal(t, 24, resolveRows(0))
	assert.Equal(t, 50, resolveRows(50))
	assert.Equal(t, defaultScrollbackLines, resolveScrollback(0))
	assert.Equal(t, 500, resolveScrollback(500))
}

// TestSession_DefaultSizeHeader proves a create with zero dims resolves to the historical
// 80×24 default and the default scrollback depth, surfaced in the CRWB1 header.
func TestSession_DefaultSizeHeader(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-defsize", "/bin/sh", dir, "", os.Environ(), 0, 0, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	blob, _ := s.Snapshot()
	assert.True(t, strings.HasPrefix(string(blob), "CRWB1 80 24 0 10000\n"),
		"default create must persist an 80×24 CRWB1 header with the default scrollback; got %q",
		firstLine(blob))
}

// TestSession_SnapshotChangeTracking proves Snapshot consumes the dirty bit: the first call
// reports changed, an immediate second call (no new output) reuses the cache and reports
// unchanged, and new output flips it back to changed.
func TestSession_SnapshotChangeTracking(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-change", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	b1, c1 := s.Snapshot()
	require.True(t, c1, "first snapshot after prompt output must report changed")

	b2, c2 := s.Snapshot()
	assert.False(t, c2, "an immediate re-snapshot with no new output must reuse the cache")
	assert.Equal(t, b1, b2, "the reused blob must be byte-identical to the cached one")

	// runShell returns only once the command's output has been through the pump AND the prompt
	// is back, so the dirty bit and the new screen content are already committed — the snapshot
	// below reads a settled state instead of re-polling one into existence.
	runShell(t, s, "echo changedmarker")

	b3, c3 := s.Snapshot()
	assert.True(t, c3, "new output must flip the session dirty again")
	assert.Contains(t, string(b3), "changedmarker",
		"the re-serialized snapshot must reflect the command's output")
}

// TestSession_InjectLocalSurfacesInBlobNotWire proves InjectLocal feeds a daemon notice into
// the model (it appears in the next Snapshot) without ever fanning it out to a live client.
func TestSession_InjectLocalSurfacesInBlobNotWire(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-inject", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	// Attaching AFTER the prompt is what makes the negative below exact rather than a guess.
	// The shell is blocked on read(2), so no further PTY output exists to fan out; and Attach
	// itself flushes any pending delta and DISARMS the trailing frame-clock timer, so no
	// deferred emit is left in flight either. Once its snapshot is consumed, the client channel
	// is quiescent by construction, and can only grow again if something fans out.
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	snap, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver its serialized snapshot")
	require.True(t, snap.Snapshot)
	require.Zero(t, len(ch), "the quiesced shell must have nothing else in flight")

	s.InjectLocal([]byte("INJECTED_NOTE_42\r\n"))

	// The notice must NOT reach the live client. Fan-out is SYNCHRONOUS — fanOutLocked runs
	// inside the caller's s.mu hold and enqueues into the client's buffered channel — so had
	// InjectLocal fanned out, the frame would already be queued the instant it returned. There
	// is no window to wait out; assert on the channel directly.
	require.Zero(t, len(ch), "InjectLocal must never fan out to a live client")

	blob, changed := s.Snapshot()
	assert.True(t, changed, "InjectLocal must mark the session dirty")
	assert.Contains(t, string(blob), "INJECTED_NOTE_42",
		"the injected notice must surface in the next serialized snapshot")
}

// TestSession_ForceSuspendSnapshotPrimaryWithNotice proves the fused force-suspend path
// drives the model to the PRIMARY buffer, injects the notice there, and returns a clean
// CRWB1 blob — never a frozen alt grid.
func TestSession_ForceSuspendSnapshotPrimaryWithNotice(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-force", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	// Drive the model into the alt buffer via injected bytes (model-only is sufficient to
	// flip the shadow/emulator), then force-suspend and assert the captured blob is primary.
	s.InjectLocal([]byte("\x1b[?1049h"))

	blob := s.ForceSuspendSnapshot([]byte("\r\n[crowbar] SUSPEND_NOTE_9\r\n"))
	assert.True(t, strings.HasPrefix(string(blob), "CRWB1 "), "force-suspend blob must carry a header")
	assert.Contains(t, string(blob), "SUSPEND_NOTE_9", "force-suspend blob must contain the notice")
	assert.NotContains(t, string(blob), "\x1b[?1049h",
		"force-suspend blob must be PRIMARY (no alt-screen enter)")
	assert.Equal(t, "0", altBit(blob), "the CRWB1 header alt flag must be 0 (primary)")
}

// TestSession_DropCachedBlobReclaimsAndDirties proves DropCachedBlob frees the cached blob
// bytes and marks the session dirty so the next Snapshot re-serializes.
func TestSession_DropCachedBlobReclaimsAndDirties(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-drop", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	b1, _ := s.Snapshot() // populates the cache
	require.NotEmpty(t, b1)

	n := s.DropCachedBlob()
	assert.Equal(t, int64(len(b1)), n, "DropCachedBlob must report the reclaimed byte count")

	_, changed := s.Snapshot()
	assert.True(t, changed, "after dropping the cache the next snapshot must re-serialize")

	// A second drop with the cache already populated again is fine; a placeholder reclaims 0.
	ph := NewPlaceholder("ph-drop", "/bin/sh", "/tmp", "", []byte("CRWB1 80 24 0 10000\n"))
	assert.Zero(t, ph.DropCachedBlob(), "a placeholder has no cache to drop")
}

func firstLine(
	b []byte,
) string {
	if i := strings.IndexByte(string(b), '\n'); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

func altBit(
	blob []byte,
) string {
	fields := strings.Fields(firstLine(blob))
	if len(fields) < 4 {
		return ""
	}
	return fields[3]
}
