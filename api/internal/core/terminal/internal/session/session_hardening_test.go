package session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func closedFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(os.DevNull)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f
}

func TestSession_Write_PTYError(t *testing.T) {
	s := newBareSession("sid-write-err", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t)

	err := s.Write([]byte("data"))
	require.Error(t, err, "Write to a closed PTY must return an error")
	assert.Contains(t, err.Error(), "session: write:")
}

func TestSession_Resize_SetsizeError(t *testing.T) {
	s := newBareSession("sid-resize-err", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t)

	err := s.Resize(80, 24)
	require.Error(t, err, "Resize on a closed PTY must return the Setsize error")
	assert.Contains(t, err.Error(), "session: resize:")
}

func TestSession_NewRestored_BadShell(t *testing.T) {
	blob := []byte("CRWB1 80 24 0 10000\nprior screen")
	s, err := NewRestored(t.Context(), "sid-restore-bad", "/nonexistent/shell/binary", t.TempDir(), "", os.Environ(), blob)
	assert.Nil(t, s, "a failed restore spawn must return a nil session")
	assert.Error(t, err, "NewRestored must surface the spawn error")
}

func TestSession_WriteModelPanicRecovered(t *testing.T) {
	ctl := &panicCtl{}
	ctl.write.Store(true)
	fm := &fakeModel{ctl: ctl, cols: 80, rows: 24}

	s := newBareSession("sid-writepanic", "/bin/sh", t.TempDir(), "")
	s.model = fm

	_, before := s.Health()
	require.NotPanics(t, func() {
		s.PumpChunkForTest([]byte("anything"))
	}, "a model Write panic must be recovered inside pumpStep")
	_, after := s.Health()
	assert.Greater(t, after, before, "a recovered model-Write panic must bump modelPanics")
}

func TestSession_InjectLocal_ExitsAltBeforeNotice(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-inject-alt", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	s.InjectLocal([]byte("\x1b[?1049h"))

	s.InjectLocal([]byte("ALT_EXIT_NOTE\r\n"))

	blob, _ := s.Snapshot()
	assert.Equal(t, "0", altBit(blob),
		"after injecting into an alt model the serialized header must be primary (alt-bit 0)")
	assert.NotContains(t, string(blob), "\x1b[?1049h",
		"the serialized primary blob must not re-enter the alt screen")
	assert.Contains(t, string(blob), "ALT_EXIT_NOTE",
		"the injected notice must survive the alt-exit and surface in the primary blob")
}

func TestSession_SuspendEligible_Force(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-bfs1", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	assert.True(t, s.SuspendEligible(true), "force-suspend of a detached session is allowed")

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	assert.False(t, s.SuspendEligible(true), "force-suspend must be rejected while a client is attached")
}

func TestParseLastOSC7_Branches(t *testing.T) {
	_, ok := parseLastOSC7([]byte("\x1b]7;file:///never/terminated"))
	assert.False(t, ok, "an unterminated OSC 7 must not yield a path")

	_, ok = parseLastOSC7([]byte("\x1b]7;http://example.com/x\x07"))
	assert.False(t, ok, "a non-file:// OSC 7 URI must be skipped")

	_, ok = parseLastOSC7([]byte("\x1b]7;file://\x07"))
	assert.False(t, ok, "a file:// URI with no path must be skipped")

	_, ok = parseLastOSC7([]byte("\x1b]7;file://h/%zz\x07"))
	assert.False(t, ok, "a malformed file:// URI must be skipped")

	path, ok := parseLastOSC7([]byte("\x1b]7;file:///st/terminated\x1b\\"))
	require.True(t, ok, "an ST-terminated file:// OSC 7 must decode")
	assert.Equal(t, "/st/terminated", path)
}

func TestIsIdleLocked_NotLive(t *testing.T) {
	ph := newBareSession("ph-idle", "/bin/sh", "/tmp", "")
	assert.False(t, ph.IsIdle(), "a session with no live PTY must report not-idle")
}
