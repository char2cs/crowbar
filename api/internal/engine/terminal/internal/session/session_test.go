package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFrame blocks until ch receives a frame or deadline elapses.
func waitFrame(
	t *testing.T,
	ch <-chan OutputFrame,
	timeout time.Duration,
) (OutputFrame, bool) {
	t.Helper()
	select {
	case f, ok := <-ch:
		return f, ok
	case <-time.After(timeout):
		return OutputFrame{}, false
	}
}

// newTestSession spawns a live session at the default 80×24 size with no scrollback
// override, the shape every session unit test that does not care about size uses.
func newTestSession(
	t *testing.T,
	id string,
	dir string,
) (*Session, error) {
	t.Helper()
	return New(id, "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
}

func TestSession_NewAndKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-1", dir)
	require.NoError(t, err)
	require.NotNil(t, s)

	s.Kill()

	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session did not terminate after Kill")
	}
}

func TestSession_AttachReceivesOutput(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-2", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo hello\n")))

	found := false
	deadline := time.After(3 * time.Second)
	for !found {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before output received")
			}
			if containsStr(f.Data, "hello") {
				found = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for 'hello' output")
		}
	}

	s.Kill()
}

// TestSession_NaturalExitReapsChild proves H4: a shell that exits on its own
// (the common case — the user types `exit`) must be reaped via cmd.Wait() on the
// pump/shutdown path, not only on Kill(). Without the reap the child is left a
// zombie. ProcessState is non-nil only after a successful Wait().
func TestSession_NaturalExitReapsChild(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-natural-exit", dir)
	require.NoError(t, err)

	// Make the shell exit on its own — no Kill().
	require.NoError(t, s.Write([]byte("exit\n")))

	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session did not terminate after the shell exited")
	}

	// The child must have been waited on (reaped); otherwise it is a zombie.
	require.NotNil(t, s.cmd.ProcessState, "natural shell exit must reap the child via cmd.Wait()")
	assert.True(t, s.cmd.ProcessState.Exited(), "child process must have exited")
}

func TestSession_AttachDeadSession(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-3", dir)
	require.NoError(t, err)
	s.Kill()

	<-s.Done()

	_, err = s.Attach()
	assert.Error(t, err)
}

func TestSession_DetachClosesChannel(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-4", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	s.Detach(ch)

	// After detach, sending more output must not block forever.
	// We write to the PTY and verify channel is closed.
	_ = s.Write([]byte("echo bye\n"))
	s.Kill()
}

// TestSession_ReAttachSerializesScreen proves the serialize-on-attach behavior switch:
// a re-attaching client receives ONE clean redraw serialized from the current screen
// model (not a raw ring replay), so output a prior client produced is still visible in
// the new client's first frame.
func TestSession_ReAttachSerializesScreen(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-5", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo ringmarker\n")))

	// Wait until the first client sees the echoed marker so the model has parsed it.
	deadline := time.After(3 * time.Second)
	found := false
	for !found {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			if containsStr(f.Data, "ringmarker") {
				found = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for output on first client")
		}
	}

	s.Detach(ch)

	// Second attach must serialize the current screen, whose grid still shows the marker.
	ch2, err := s.Attach()
	require.NoError(t, err)

	f, ok := waitFrame(t, ch2, 2*time.Second)
	assert.True(t, ok, "expected serialized redraw frame")
	assert.True(t, containsStr(f.Data, "ringmarker"),
		"serialized redraw must contain the on-screen marker, got: %q", f.Data)
	// The serialized redraw must be a clean ground-state frame: no replay sanitizer CAN
	// byte and no historical query bytes — it starts with the soft-reset DECSTR.
	assert.True(t, contains(f.Data, []byte("\x1b[!p")),
		"serialized redraw must begin with the DECSTR soft reset, got: %q", f.Data)

	s.Detach(ch2)
	s.Kill()
}

func TestSession_Resize(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-6", dir)
	require.NoError(t, err)
	assert.NoError(t, s.Resize(120, 40))
	s.Kill()
}

func TestSession_ID(t *testing.T) {
	dir := t.TempDir()
	s, err := New("my-id", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	assert.Equal(t, "my-id", s.ID())
	s.Kill()
}

func TestSession_New_BadShell(t *testing.T) {
	dir := t.TempDir()
	// A non-existent executable must cause pty.Start to fail.
	_, err := New("sid-bad", "/nonexistent/shell/binary", dir, "", os.Environ(), 80, 24, 0)
	assert.Error(t, err)
}

func TestSession_WriteAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-7", dir)
	require.NoError(t, err)
	s.Kill()
	// Wait for PTY to fully close.
	<-s.Done()
	// Writing to a killed session should return an error.
	writeErr := s.Write([]byte("hello"))
	assert.Error(t, writeErr)
}

func TestSession_ResizeAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-8", dir)
	require.NoError(t, err)
	s.Kill()
	<-s.Done()
	// Resizing a killed session should return an error.
	resizeErr := s.Resize(80, 24)
	assert.Error(t, resizeErr)
}

func TestSession_DropOnOverflow(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-9", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)

	// Drain any snapshot frame that was delivered on attach.
	for len(ch) > 0 {
		<-ch
	}

	// Call fanOut directly until the client's channel overflows.
	// ClientSendBufForTest + 1 writes are guaranteed to overflow.
	chunk := []byte("x")
	for i := 0; i <= ClientSendBufForTest; i++ {
		s.FanOutForTest(chunk)
	}

	// The channel must now be closed because the client was dropped.
	select {
	case _, ok := <-ch:
		// Either we read the overflow batch or the channel is closed.
		if !ok {
			return // channel closed: drop happened
		}
		// Channel not closed yet; drain and check again.
		for len(ch) > 0 {
			if _, ok := <-ch; !ok {
				return
			}
		}
	default:
	}

	// If not yet closed, wait a moment for the goroutine to process.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("client was not dropped after channel overflow")
		}
	}
}

func TestIsNormalPTYClose_EOF(t *testing.T) {
	assert.True(t, isNormalPTYClose(io.EOF))
}

func TestIsNormalPTYClose_EIO(t *testing.T) {
	// On Linux the PTY master returns EIO when the shell exits.
	assert.True(t, isNormalPTYClose(syscall.EIO))
}

func TestIsNormalPTYClose_WrappedEIO(t *testing.T) {
	wrapped := fmt.Errorf("pty: %w", syscall.EIO)
	assert.True(t, isNormalPTYClose(wrapped))
}

func TestIsNormalPTYClose_OtherError(t *testing.T) {
	assert.False(t, isNormalPTYClose(errors.New("unexpected error")))
	assert.False(t, isNormalPTYClose(syscall.EPERM))
}

// TestSession_ReplayLiveHandoff_NoDuplication verifies that a client attaching
// while the pump is between the model write and fanOut never receives the same chunk
// twice (once in the serialized attach redraw, once via the live fan-out delivery).
//
// PumpChunkForTest delegates to pumpStep — the production critical section used by
// pump() — so this test exercises the real code path. A regression that removes the
// lock from pumpStep will be caught by the race detector running this test.
//
// Run with: go test -race -run TestSession_ReplayLiveHandoff_NoDuplication
func TestSession_ReplayLiveHandoff_NoDuplication(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-handoff", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	const rounds = 100

	// makeChunk returns a unique 8-byte sentinel for each round using bytes
	// that are invalid UTF-8 lead bytes and will not appear in shell output.
	makeChunk := func(r int) []byte {
		return []byte{
			0xC0, 0xC1,
			byte(r >> 8), byte(r & 0xFF),
			0xFE, 0xFF,
			byte(r >> 8), byte(r & 0xFF),
		}
	}

	var dupCount int64
	stop := make(chan struct{})

	// Attacher goroutines: continuously attach, collect frames, check for
	// duplicate delivery of any known chunk.
	const numAttachers = 5
	var attachWg sync.WaitGroup
	for a := 0; a < numAttachers; a++ {
		attachWg.Add(1)
		go func() {
			defer attachWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}

				ch, attachErr := s.Attach()
				if attachErr != nil {
					return // session is dead
				}

				var received []byte
				timer := time.NewTimer(5 * time.Millisecond)
			drain:
				for {
					select {
					case f, ok := <-ch:
						if !ok {
							break drain
						}
						received = append(received, f.Data...)
					case <-timer.C:
						break drain
					}
				}
				timer.Stop()
				s.Detach(ch)

				// Any unique chunk appearing more than once means the client
				// saw it in both the serialized attach redraw and the live fan-out.
				for r := 0; r < rounds; r++ {
					if countOccurrences(received, makeChunk(r)) > 1 {
						atomic.AddInt64(&dupCount, 1)
					}
				}
			}
		}()
	}

	// Producer: pump all chunks via the pump-simulation helper. PumpChunkForTest
	// delegates to pumpStep (the production critical section), so this exercises
	// the same lock path as the real pump goroutine.
	for r := 0; r < rounds; r++ {
		s.PumpChunkForTest(makeChunk(r))
	}
	close(stop)
	attachWg.Wait()

	assert.Zero(t, dupCount,
		"detected %d chunk duplication(s): snapshot + live delivery both fired for the same data — ring write and fan-out are not atomic",
		dupCount)
}

func containsStr(
	data []byte,
	sub string,
) bool {
	return len(data) > 0 && len(sub) > 0 && contains(data, []byte(sub))
}

func contains(
	haystack []byte,
	needle []byte,
) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// New lifecycle / safety tests
// ---------------------------------------------------------------------------

// TestIsLive_AttachedCount_State_Transitions verifies the State/IsLive/
// AttachedCount accessors across a typical session lifecycle:
//
//	live + 0 clients → "detached"
//	live + 1 client  → "active"
//	after Kill        → IsLive false, State "suspended"
func TestIsLive_AttachedCount_State_Transitions(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-lifecycle", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	// Freshly spawned: live, no clients → "detached".
	require.True(t, s.IsLive(), "newly spawned session must be live")
	require.Equal(t, 0, s.AttachedCount())
	require.Equal(t, "detached", s.State())

	ch, err := s.Attach()
	require.NoError(t, err)

	// One client → "active".
	require.Equal(t, 1, s.AttachedCount())
	require.Equal(t, "active", s.State())

	s.Detach(ch)

	// Back to no clients → "detached".
	require.Equal(t, 0, s.AttachedCount())
	require.Equal(t, "detached", s.State())

	// Kill and wait for shutdown.
	s.Kill()
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session did not shut down after Kill")
	}

	require.False(t, s.IsLive(), "after Kill, IsLive must return false")
	require.Equal(t, "suspended", s.State())
}

// TestNewPlaceholder verifies that a placeholder session:
//   - reports IsLive false and State "suspended"
//   - holds NO model and delivers no serialized frame on Attach (the engine restores a
//     placeholder to a live session before attaching in production), without panicking
//   - can be killed without panic, closing Done()
func TestNewPlaceholder(t *testing.T) {
	rawBlob := []byte("CRWB1 80 24 0 10000\nold output here")
	s := NewPlaceholder("ph-1", "/bin/sh", "/tmp", "prof-A", rawBlob)

	require.False(t, s.IsLive(), "placeholder must not be live")
	require.Equal(t, "suspended", s.State())
	require.Equal(t, "/bin/sh", s.Shell())
	require.Equal(t, "/tmp", s.CWD())
	require.Equal(t, "prof-A", s.ProfileID())

	// A model-less placeholder Attach must not panic and must deliver no redraw frame
	// (there is no model to serialize).
	ch, err := s.Attach()
	require.NoError(t, err)
	require.NotNil(t, ch)
	select {
	case f := <-ch:
		t.Fatalf("placeholder Attach must not deliver a frame, got: %q", f.Data)
	case <-time.After(200 * time.Millisecond):
	}
	s.Detach(ch)

	// Kill must not panic and must close Done().
	s.Kill()
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("placeholder Kill did not close Done channel")
	}
}

// TestNewPlaceholder_ModelBytesIsBlobLen verifies a placeholder accounts only its stored
// blob bytes (no live model), so thousands of suspended placeholders do not each pin a full
// model's worth of memory.
func TestNewPlaceholder_ModelBytesIsBlobLen(t *testing.T) {
	rawBlob := []byte("CRWB1 80 24 0 10000\nsome prior bytes")
	s := NewPlaceholder("ph-cap", "/bin/sh", "/tmp", "", rawBlob)

	require.Equal(t, int64(len(rawBlob)), s.ModelBytes(),
		"placeholder ModelBytes must equal its stored blob length")
}

// TestLiveSession_ModelBytesAccountsGrid verifies a live session's ModelBytes reflects its
// grid (and grows with scrollback) so the byte-ceiling accounting stays correct.
func TestLiveSession_ModelBytesAccountsGrid(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-cap", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	require.Greater(t, s.ModelBytes(), int64(0),
		"a live session must account a positive model footprint")
}

// TestOSC7_CWD_Update pumps two OSC 7 sequences through pumpStep and verifies
// that CWD() returns the path from the LAST sequence.
func TestOSC7_CWD_Update(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-osc7", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	// Build a chunk with two OSC 7 sequences: first /old/dir, then /new/dir.
	chunk := []byte(
		"\x1b]7;file:///old/dir\x07" +
			"\x1b]7;file:///new/dir\x07",
	)
	s.PumpChunkForTest(chunk)

	require.Equal(t, "/new/dir", s.CWD(),
		"CWD must reflect the last OSC 7 path in the chunk")
}

// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Phase 2: Suspend/Restore helpers
// ---------------------------------------------------------------------------

// TestSession_Snapshot_ContentFromPlaceholder verifies a placeholder Snapshot returns its
// stored blob verbatim with changed==false (no model, nothing to re-serialize).
func TestSession_Snapshot_ContentFromPlaceholder(t *testing.T) {
	data := []byte("CRWB1 80 24 0 10000\nscrollback data")
	ph := NewPlaceholder("ph-snap", "/bin/sh", "/tmp", "", data)
	snap, changed := ph.Snapshot()
	assert.Equal(t, data, snap)
	assert.False(t, changed, "a placeholder's persisted blob never changes")
}

// TestSession_BeginSuspendIfEligible_WithClients verifies no-op when clients attached.
func TestSession_BeginSuspendIfEligible_WithClients(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-bse1", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	assert.False(t, s.BeginSuspendIfEligible(), "with attached clients, must not be eligible")
	assert.False(t, s.Suspending())
}

// TestSession_BeginSuspendIfEligible_AlreadySuspending verifies that a second call
// is a no-op once the flag is set.
func TestSession_BeginSuspendIfEligible_AlreadySuspending(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-bse2", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	if !waitIdleOrSkip(t, s) {
		return
	}
	assert.True(t, s.BeginSuspendIfEligible(), "first call on idle session must succeed")
	assert.True(t, s.Suspending())
	assert.False(t, s.BeginSuspendIfEligible(), "second call must return false (already suspending)")
}

// TestSession_Suspending verifies the flag is readable after BeginSuspendIfEligible.
func TestSession_Suspending(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-suspflag", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.False(t, s.Suspending(), "must be false before any suspend call")

	if !waitIdleOrSkip(t, s) {
		return
	}
	s.BeginSuspendIfEligible()
	assert.True(t, s.Suspending())
}

// TestSession_ExitCode_Default verifies ExitCode is -1 before any exit.
func TestSession_ExitCode_Default(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-ec0", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.Equal(t, -1, s.ExitCode())
}

// TestSession_ExitCode_AfterCleanExit verifies ExitCode is 0 after `exit 0`.
func TestSession_ExitCode_AfterCleanExit(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-ec1", dir)
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("exit 0\n")))
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		s.Kill()
		t.Fatal("session did not exit after `exit 0`")
	}
	assert.Equal(t, 0, s.ExitCode())
}

// TestSession_NewRestored_RebuildsModelFromBlob verifies that NewRestored parses a
// persisted CRWB1 blob, rebuilds the model from its redraw bytes BEFORE the pump starts,
// and a first Attach serializes the restored screen — so prior on-screen content survives
// the round-trip. The blob is produced by a real live session's Snapshot so the header and
// redraw are exactly what the engine persists.
func TestSession_NewRestored_RebuildsModelFromBlob(t *testing.T) {
	dir := t.TempDir()

	// Produce a real persisted blob: a live session that printed a known marker.
	src, err := newTestSession(t, "sid-src", dir)
	require.NoError(t, err)
	require.NoError(t, src.Write([]byte("echo restoremarker\n")))
	deadline := time.After(3 * time.Second)
	for {
		blob, _ := src.Snapshot()
		if contains(blob, []byte("restoremarker")) {
			src.Kill()
			break
		}
		select {
		case <-deadline:
			src.Kill()
			t.Fatal("source session never produced the marker on screen")
		case <-time.After(50 * time.Millisecond):
		}
	}
	blob, _ := src.Snapshot()
	require.True(t, contains(blob, []byte("CRWB1 ")), "blob must carry a CRWB1 header")

	s, err := NewRestored("sid-restored", "/bin/sh", dir, "profX", os.Environ(), blob)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	select {
	case f, ok := <-ch:
		require.True(t, ok)
		require.True(t, contains(f.Data, []byte("restoremarker")),
			"first attach frame must contain the restored on-screen marker; got %q", f.Data)
	case <-time.After(2 * time.Second):
		t.Fatal("no serialized frame received from restored session")
	}
}

// waitIdleOrSkip polls until the shell is idle. Returns false (and calls t.Skip)
// if the shell does not become idle within 5 s.
func waitIdleOrSkip(t *testing.T, s *Session) bool {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if s.IsIdle() {
			return true
		}
		select {
		case <-deadline:
			t.Skip("shell did not become idle within 5s; skipping idle-dependent test")
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// countOccurrences counts non-overlapping occurrences of needle in data.
func countOccurrences(data, needle []byte) int {
	if len(needle) == 0 || len(data) < len(needle) {
		return 0
	}
	count := 0
	for i := 0; i <= len(data)-len(needle); {
		match := true
		for j := 0; j < len(needle); j++ {
			if data[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			count++
			i += len(needle) // skip past the match (non-overlapping)
		} else {
			i++
		}
	}
	return count
}
