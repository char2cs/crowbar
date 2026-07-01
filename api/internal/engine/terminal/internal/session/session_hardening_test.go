package session

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
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

// swapPtyForegroundPgid replaces the foreground-pgid probe seam for the test's duration.
func swapPtyForegroundPgid(t *testing.T, fn func(uintptr) (int, error)) {
	t.Helper()
	orig := ptyForegroundPgid
	ptyForegroundPgid = fn
	t.Cleanup(func() { ptyForegroundPgid = orig })
}

// swapGetpgidFn replaces the getpgid seam for the test's duration.
func swapGetpgidFn(t *testing.T, fn func(int) (int, error)) {
	t.Helper()
	orig := getpgidFn
	getpgidFn = fn
	t.Cleanup(func() { getpgidFn = orig })
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
	s, err := NewRestored("sid-restore-bad", "/nonexistent/shell/binary", t.TempDir(), "", os.Environ(), blob)
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

// TestSession_InjectLocal_Placeholder covers injectLocalLocked's model==nil guard: injecting
// into a placeholder (no model) is a no-op and never changes its persisted blob.
func TestSession_InjectLocal_Placeholder(t *testing.T) {
	raw := []byte("CRWB1 80 24 0 10000\nplaceholder body")
	ph := NewPlaceholder("ph-inject", "/bin/sh", "/tmp", "", raw)

	ph.InjectLocal([]byte("ignored notice"))

	blob, changed := ph.Snapshot()
	assert.Equal(t, raw, blob, "InjectLocal on a placeholder must not mutate its blob")
	assert.False(t, changed)
}

// TestSession_InjectLocal_ExitsAltBeforeNotice covers injectLocalLocked's alt-screen exit
// branch: when the model is in the alt buffer at injection time, the notice is preceded by
// the alt-exit sequence so the daemon notice can never land in a transient alt buffer.
func TestSession_InjectLocal_ExitsAltBeforeNotice(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-inject-alt", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	settle(t, s)

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

// TestSession_ForceSuspendSnapshot_Placeholder covers ForceSuspendSnapshot's model==nil
// fast-path: a placeholder returns its stored blob verbatim.
func TestSession_ForceSuspendSnapshot_Placeholder(t *testing.T) {
	raw := []byte("CRWB1 80 24 0 10000\nsuspended already")
	ph := NewPlaceholder("ph-force", "/bin/sh", "/tmp", "", raw)

	got := ph.ForceSuspendSnapshot([]byte("\r\nnotice\r\n"))
	assert.Equal(t, raw, got, "a placeholder ForceSuspendSnapshot must return its stored blob verbatim")
}

// TestSession_BeginForceSuspend_Guards covers BeginForceSuspend's reject branch: it succeeds
// once on a detached live session, then returns false when already suspending, and returns
// false when a client is attached.
func TestSession_BeginForceSuspend_Guards(t *testing.T) {
	dir := t.TempDir()

	// (1) already-suspending guard.
	s, err := newTestSession(t, "sid-bfs1", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	assert.True(t, s.BeginForceSuspend(), "first force-suspend on a detached session must begin")
	assert.True(t, s.Suspending())
	assert.False(t, s.BeginForceSuspend(), "a second force-suspend must be rejected (already suspending)")

	// (2) has-clients guard.
	s2, err := newTestSession(t, "sid-bfs2", dir)
	require.NoError(t, err)
	t.Cleanup(s2.Kill)
	ch, err := s2.Attach()
	require.NoError(t, err)
	defer s2.Detach(ch)
	assert.False(t, s2.BeginForceSuspend(), "force-suspend must be rejected while a client is attached")
}

// TestSession_SerializedLen_Placeholder covers SerializedLen's model==nil branch: a
// placeholder reports its stored blob length.
func TestSession_SerializedLen_Placeholder(t *testing.T) {
	raw := []byte("CRWB1 80 24 0 10000\nstored")
	ph := NewPlaceholder("ph-len", "/bin/sh", "/tmp", "", raw)
	assert.Equal(t, len(raw), ph.SerializedLen(),
		"a placeholder SerializedLen must equal its stored blob length")
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

// TestPtyForegroundPgid_IoctlError covers the ptyForegroundPgid errno branch: a closed fd
// makes the TIOCGPGRP ioctl fail, returning an error.
func TestPtyForegroundPgid_IoctlError(t *testing.T) {
	f := closedFile(t)
	_, err := ptyForegroundPgid(f.Fd())
	require.Error(t, err, "TIOCGPGRP on a closed fd must return an error")
}

// fgSession builds a non-live-PTY-pump bare session that nonetheless passes the
// ptmx/cmd/Process non-nil guards in isIdleLocked/checkForegroundResetLocked, so the syscall
// seams decide the outcome.
func fgSession(t *testing.T) *Session {
	t.Helper()
	s := newBareSession("sid-fg", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t)
	s.cmd = &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	s.model = &fakeModel{cols: 80, rows: 24}
	return s
}

// TestIsIdleLocked_NotLive covers the not-live guard: a placeholder (no ptmx) is never idle.
func TestIsIdleLocked_NotLive(t *testing.T) {
	ph := NewPlaceholder("ph-idle", "/bin/sh", "/tmp", "", nil)
	assert.False(t, ph.IsIdle(), "a placeholder (no live PTY) must report not-idle")
}

// TestIsIdleLocked_IoctlError covers isIdleLocked's ioctl-error branch via the seam.
func TestIsIdleLocked_IoctlError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 0, errors.New("ioctl boom") })
	s := fgSession(t)
	assert.False(t, s.IsIdle(), "an ioctl error must make isIdleLocked report not-idle")
}

// TestIsIdleLocked_GetpgidError covers isIdleLocked's getpgid-error branch via the seam.
func TestIsIdleLocked_GetpgidError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 4242, nil })
	swapGetpgidFn(t, func(int) (int, error) { return 0, errors.New("getpgid boom") })
	s := fgSession(t)
	assert.False(t, s.IsIdle(), "a getpgid error must make isIdleLocked report not-idle")
}

// TestCheckForegroundResetLocked_IoctlError covers checkForegroundResetLocked's ioctl-error
// early return: the model's OnForegroundReset must NOT fire when the probe fails.
func TestCheckForegroundResetLocked_IoctlError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 0, errors.New("ioctl boom") })
	s := fgSession(t)
	rec := &resetRecorder{}
	s.model = rec
	s.mu.Lock()
	s.checkForegroundResetLocked()
	s.mu.Unlock()
	assert.False(t, rec.reset, "an ioctl error must skip OnForegroundReset")
}

// TestCheckForegroundResetLocked_GetpgidError covers checkForegroundResetLocked's
// getpgid-error early return.
func TestCheckForegroundResetLocked_GetpgidError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 4242, nil })
	swapGetpgidFn(t, func(int) (int, error) { return 0, errors.New("getpgid boom") })
	s := fgSession(t)
	rec := &resetRecorder{}
	s.model = rec
	s.mu.Lock()
	s.checkForegroundResetLocked()
	s.mu.Unlock()
	assert.False(t, rec.reset, "a getpgid error must skip OnForegroundReset")
}

// resetRecorder is an inert model that records whether OnForegroundReset fired.
type resetRecorder struct {
	fakeModel
	reset bool
}

func (m *resetRecorder) OnForegroundReset() { m.reset = true }

var _ model.TerminalModel = (*resetRecorder)(nil)
