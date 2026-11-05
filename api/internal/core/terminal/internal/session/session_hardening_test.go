package session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedFile returns a non-nil but already-closed *os.File so a Write/ioctl against it fails
// with a real OS error, without spawning a PTY or a pump goroutine.
func closedFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(os.DevNull)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f
}

// TestSession_Write_PTYError covers the Write error path: a live ptmx whose underlying file
// is closed makes ptmx.Write fail, and Write must wrap and return that error (not panic).
func TestSession_Write_PTYError(t *testing.T) {
	s := newBareSession("sid-write-err", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t) // non-nil but closed → ptmx.Write returns an error

	err := s.Write([]byte("data"))
	require.Error(t, err, "Write to a closed PTY must return an error")
	assert.Contains(t, err.Error(), "session: write:")
}

// TestSession_Resize_SetsizeError covers the Resize error path: a live ptmx whose file is
// closed makes pty.Setsize (TIOCSWINSZ) fail, and Resize must return that error before it
// touches the model.
func TestSession_Resize_SetsizeError(t *testing.T) {
	s := newBareSession("sid-resize-err", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t)

	err := s.Resize(80, 24)
	require.Error(t, err, "Resize on a closed PTY must return the Setsize error")
	assert.Contains(t, err.Error(), "session: resize:")
}

// TestSession_NewRestored_BadShell covers NewRestored's spawn-error path: a non-existent
// shell binary makes pty.Start fail, so NewRestored returns a nil session and an error.
func TestSession_NewRestored_BadShell(t *testing.T) {
	blob := []byte("CRWB1 80 24 0 10000\nprior screen")
	s, err := NewRestored(t.Context(), "sid-restore-bad", "/nonexistent/shell/binary", t.TempDir(), "", os.Environ(), blob)
	assert.Nil(t, s, "a failed restore spawn must return a nil session")
	assert.Error(t, err, "NewRestored must surface the spawn error")
}

// TestSession_WriteModelPanicRecovered covers the writeModelLocked recover backstop: a model
// whose Write panics must be recovered inside pumpStep (bumping modelPanics) so the panic
// never escapes the critical section.
func TestSession_WriteModelPanicRecovered(t *testing.T) {
	ctl := &panicCtl{}
	ctl.write.Store(true)
	fm := &fakeModel{ctl: ctl, cols: 80, rows: 24}

	s := newBareSession("sid-writepanic", "/bin/sh", t.TempDir(), "")
	s.model = fm // no PTY, no pump: PumpChunkForTest is the only thing that touches pumpStep

	_, before := s.Health()
	require.NotPanics(t, func() {
		s.PumpChunkForTest([]byte("anything"))
	}, "a model Write panic must be recovered inside pumpStep")
	_, after := s.Health()
	assert.Greater(t, after, before, "a recovered model-Write panic must bump modelPanics")
}

// TestSession_InjectLocal_ExitsAltBeforeNotice covers injectLocalLocked's alt-screen exit
// branch: when the model is in the alt buffer at injection time, the notice is preceded by
// the alt-exit sequence so the daemon notice can never land in a transient alt buffer.
func TestSession_InjectLocal_ExitsAltBeforeNotice(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-inject-alt", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	waitPrompt(t, s)

	// First inject drives the model INTO the alt buffer (model is primary at this point, so
	// the alt-exit branch is NOT taken here).
	s.InjectLocal([]byte("\x1b[?1049h"))

	// Second inject finds the model in alt → injectLocalLocked must emit the alt-exit
	// preamble before the notice, leaving a primary screen carrying the notice.
	s.InjectLocal([]byte("ALT_EXIT_NOTE\r\n"))

	blob, _ := s.Snapshot()
	assert.Equal(t, "0", altBit(blob),
		"after injecting into an alt model the serialized header must be primary (alt-bit 0)")
	assert.NotContains(t, string(blob), "\x1b[?1049h",
		"the serialized primary blob must not re-enter the alt screen")
	assert.Contains(t, string(blob), "ALT_EXIT_NOTE",
		"the injected notice must survive the alt-exit and surface in the primary blob")
}

// TestSession_SuspendEligible_Force covers the force flag: a detached session that is not
// at an idle prompt is eligible only by force, and never while a client is attached.
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

// TestParseLastOSC7_Branches covers the non-happy parseLastOSC7 paths: an unterminated
// sequence (no BEL/ST), a non-file:// URI, and a file:// URI with an empty/invalid path —
// each of which must yield no CWD update — plus the ST-terminated happy path.
func TestParseLastOSC7_Branches(t *testing.T) {
	// Unterminated: lead-in present but no BEL/ST terminator → no match.
	_, ok := parseLastOSC7([]byte("\x1b]7;file:///never/terminated"))
	assert.False(t, ok, "an unterminated OSC 7 must not yield a path")

	// Non-file scheme → skipped.
	_, ok = parseLastOSC7([]byte("\x1b]7;http://example.com/x\x07"))
	assert.False(t, ok, "a non-file:// OSC 7 URI must be skipped")

	// file:// with empty path → skipped.
	_, ok = parseLastOSC7([]byte("\x1b]7;file://\x07"))
	assert.False(t, ok, "a file:// URI with no path must be skipped")

	// file:// with an invalid percent-escape → url.Parse errors → skipped.
	_, ok = parseLastOSC7([]byte("\x1b]7;file://h/%zz\x07"))
	assert.False(t, ok, "a malformed file:// URI must be skipped")

	// ST-terminated (ESC backslash) happy path → path decoded.
	path, ok := parseLastOSC7([]byte("\x1b]7;file:///st/terminated\x1b\\"))
	require.True(t, ok, "an ST-terminated file:// OSC 7 must decode")
	assert.Equal(t, "/st/terminated", path)
}

// TestIsIdleLocked_NotLive covers the not-live guard: a session with no PTY is never idle.
func TestIsIdleLocked_NotLive(t *testing.T) {
	ph := newBareSession("ph-idle", "/bin/sh", "/tmp", "")
	assert.False(t, ph.IsIdle(), "a session with no live PTY must report not-idle")
}
