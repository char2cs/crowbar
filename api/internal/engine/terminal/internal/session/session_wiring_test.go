package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settle waits until the session's serialized screen length stops changing for a short
// window — a proxy for "the shell has printed its prompt and gone quiet" — WITHOUT consuming
// the dirty bit (SerializedLen is a pure read).
func settle(
	t *testing.T,
	s *Session,
) {
	t.Helper()
	deadline := time.After(8 * time.Second)
	last := -1
	stable := 0
	for {
		cur := s.SerializedLen()
		if cur == last {
			stable++
			if stable >= 5 {
				return
			}
		} else {
			stable = 0
			last = cur
		}
		select {
		case <-deadline:
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

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
	settle(t, s)

	b1, c1 := s.Snapshot()
	require.True(t, c1, "first snapshot after prompt output must report changed")

	b2, c2 := s.Snapshot()
	assert.False(t, c2, "an immediate re-snapshot with no new output must reuse the cache")
	assert.Equal(t, b1, b2, "the reused blob must be byte-identical to the cached one")

	require.NoError(t, s.Write([]byte("echo changedmarker\n")))
	deadline := time.After(3 * time.Second)
	for {
		b3, c3 := s.Snapshot()
		if c3 && strings.Contains(string(b3), "changedmarker") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("snapshot never reflected new output as changed")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestSession_InjectLocalSurfacesInBlobNotWire proves InjectLocal feeds a daemon notice into
// the model (it appears in the next Snapshot) without ever fanning it out to a live client.
func TestSession_InjectLocalSurfacesInBlobNotWire(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-inject", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	settle(t, s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	drain(ch)

	s.InjectLocal([]byte("INJECTED_NOTE_42\r\n"))

	// The notice must NOT reach the live client (InjectLocal never fans out).
	select {
	case f := <-ch:
		assert.NotContains(t, string(f.Data), "INJECTED_NOTE_42",
			"InjectLocal must never fan out to a live client")
	case <-time.After(200 * time.Millisecond):
	}

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
	settle(t, s)

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
	settle(t, s)

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

func drain(
	ch <-chan OutputFrame,
) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
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
