package session

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBlob(t *testing.T) {
	cols, rows, sb, body := parseBlob([]byte("CRWB1 100 40 1 5000\nhello world"))
	assert.Equal(t, 100, cols)
	assert.Equal(t, 40, rows)
	assert.Equal(t, 5000, sb)
	assert.Equal(t, []byte("hello world"), body)

	c, r, s, b := parseBlob([]byte("no newline anywhere"))
	assert.Zero(t, c+r+s)
	assert.Nil(t, b)

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

func TestSession_DefaultSizeHeader(t *testing.T) {
	dir := t.TempDir()
	s, err := New(t.Context(), "sid-defsize", "/bin/sh", dir, "", os.Environ(), 0, 0, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	blob, _ := s.Snapshot()
	assert.True(t, strings.HasPrefix(string(blob), "CRWB1 80 24 0 10000\n"),
		"default create must persist an 80×24 CRWB1 header with the default scrollback; got %q",
		firstLine(blob))
}

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

	runShell(t, s, "echo changedmarker")

	b3, c3 := s.Snapshot()
	assert.True(t, c3, "new output must flip the session dirty again")
	assert.Contains(t, string(b3), "changedmarker",
		"the re-serialized snapshot must reflect the command's output")
}

func TestSession_InjectLocalSurfacesInBlobNotWire(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-inject", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	snap, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver its serialized snapshot")
	require.True(t, snap.Snapshot)
	require.Zero(t, len(ch), "the quiesced shell must have nothing else in flight")

	s.InjectLocal([]byte("INJECTED_NOTE_42\r\n"))

	require.Zero(t, len(ch), "InjectLocal must never fan out to a live client")

	blob, changed := s.Snapshot()
	assert.True(t, changed, "InjectLocal must mark the session dirty")
	assert.Contains(t, string(blob), "INJECTED_NOTE_42",
		"the injected notice must surface in the next serialized snapshot")
}

func TestSession_ForceSuspendSnapshotPrimaryWithNotice(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-force", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	s.InjectLocal([]byte("\x1b[?1049h"))

	blob := s.ForceSuspendSnapshot([]byte("\r\n[crowbar] SUSPEND_NOTE_9\r\n"))
	assert.True(t, strings.HasPrefix(string(blob), "CRWB1 "), "force-suspend blob must carry a header")
	assert.Contains(t, string(blob), "SUSPEND_NOTE_9", "force-suspend blob must contain the notice")
	assert.NotContains(t, string(blob), "\x1b[?1049h",
		"force-suspend blob must be PRIMARY (no alt-screen enter)")
	assert.Equal(t, "0", altBit(blob), "the CRWB1 header alt flag must be 0 (primary)")
}

func TestSession_DropCachedBlobReclaimsAndDirties(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-drop", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	b1, _ := s.Snapshot()
	require.NotEmpty(t, b1)

	n := s.DropCachedBlob()
	assert.Equal(t, int64(len(b1)), n, "DropCachedBlob must report the reclaimed byte count")

	_, changed := s.Snapshot()
	assert.True(t, changed, "after dropping the cache the next snapshot must re-serialize")

	s.DropCachedBlob()
	assert.Zero(t, s.DropCachedBlob(), "a second drop with nothing cached reclaims 0")
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
