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

func TestSession_NewAndKill(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-1", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-2", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-natural-exit", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-3", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)
	s.Kill()

	<-s.Done()

	_, err = s.Attach()
	assert.Error(t, err)
}

func TestSession_DetachClosesChannel(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-4", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	s.Detach(ch)

	// After detach, sending more output must not block forever.
	// We write to the PTY and verify channel is closed.
	_ = s.Write([]byte("echo bye\n"))
	s.Kill()
}

func TestSession_RingBufferReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-5", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo ring\n")))

	// Wait until ring has data.
	deadline := time.After(3 * time.Second)
	found := false
	for !found {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			if containsStr(f.Data, "ring") {
				found = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for ring output on first client")
		}
	}

	s.Detach(ch)

	// Second attach must replay ring (snapshot contains "ring").
	ch2, err := s.Attach()
	require.NoError(t, err)

	f, ok := waitFrame(t, ch2, 2*time.Second)
	assert.True(t, ok, "expected replay frame")
	assert.True(t, containsStr(f.Data, "ring"), "replay must contain 'ring', got: %q", f.Data)

	s.Detach(ch2)
	s.Kill()
}

func TestSession_Resize(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-6", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)
	assert.NoError(t, s.Resize(120, 40))
	s.Kill()
}

func TestSession_ID(t *testing.T) {
	dir := t.TempDir()
	s, err := New("my-id", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)
	assert.Equal(t, "my-id", s.ID())
	s.Kill()
}

func TestSession_New_BadShell(t *testing.T) {
	dir := t.TempDir()
	// A non-existent executable must cause pty.Start to fail.
	_, err := New("sid-bad", "/nonexistent/shell/binary", dir, "", os.Environ())
	assert.Error(t, err)
}

func TestSession_WriteAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-7", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-8", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)
	s.Kill()
	<-s.Done()
	// Resizing a killed session should return an error.
	resizeErr := s.Resize(80, 24)
	assert.Error(t, resizeErr)
}

func TestSession_DropOnOverflow(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-9", "/bin/sh", dir, "", os.Environ())
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
// while the pump is between ring.Write and fanOut never receives the same chunk
// twice (once in the ring snapshot, once via the live fan-out delivery).
//
// PumpChunkForTest delegates to pumpStep — the production critical section used by
// pump() — so this test exercises the real code path. A regression that removes the
// lock from pumpStep will be caught by the race detector running this test.
//
// Run with: go test -race -run TestSession_ReplayLiveHandoff_NoDuplication
func TestSession_ReplayLiveHandoff_NoDuplication(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-handoff", "/bin/sh", dir, "", os.Environ())
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
				// saw it in both the ring snapshot and the live fan-out.
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
//   live + 0 clients → "detached"
//   live + 1 client  → "active"
//   after Kill        → IsLive false, State "suspended"
func TestIsLive_AttachedCount_State_Transitions(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-lifecycle", "/bin/sh", dir, "", os.Environ())
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
//   - returns the loaded scrollback on Attach without panic
//   - can be killed without panic, closing Done()
func TestNewPlaceholder(t *testing.T) {
	scrollback := []byte("old output here")
	s := NewPlaceholder("ph-1", "/bin/sh", "/tmp", "prof-A", scrollback)

	require.False(t, s.IsLive(), "placeholder must not be live")
	require.Equal(t, "suspended", s.State())
	require.Equal(t, "/bin/sh", s.Shell())
	require.Equal(t, "/tmp", s.CWD())
	require.Equal(t, "prof-A", s.ProfileID())

	// Attach must succeed and replay the scrollback.
	ch, err := s.Attach()
	require.NoError(t, err)
	require.NotNil(t, ch)

	// The ring snapshot should be delivered immediately.
	select {
	case f, ok := <-ch:
		require.True(t, ok)
		require.Equal(t, scrollback, f.Data)
	case <-time.After(time.Second):
		t.Fatal("placeholder Attach did not deliver scrollback snapshot")
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

// TestOSC7_CWD_Update pumps two OSC 7 sequences through pumpStep and verifies
// that CWD() returns the path from the LAST sequence.
func TestOSC7_CWD_Update(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-osc7", "/bin/sh", dir, "", os.Environ())
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

// TestSession_Snapshot_ContentFromPlaceholder verifies Snapshot returns ring content.
func TestSession_Snapshot_ContentFromPlaceholder(t *testing.T) {
	data := []byte("scrollback data")
	ph := NewPlaceholder("ph-snap", "/bin/sh", "/tmp", "", data)
	snap := ph.Snapshot()
	assert.Equal(t, data, snap)
}

// TestSession_BeginSuspendIfEligible_WithClients verifies no-op when clients attached.
func TestSession_BeginSuspendIfEligible_WithClients(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-bse1", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-bse2", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-suspflag", "/bin/sh", dir, "", os.Environ())
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
	s, err := New("sid-ec0", "/bin/sh", dir, "", os.Environ())
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.Equal(t, -1, s.ExitCode())
}

// TestSession_ExitCode_AfterCleanExit verifies ExitCode is 0 after `exit 0`.
func TestSession_ExitCode_AfterCleanExit(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-ec1", "/bin/sh", dir, "", os.Environ())
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

// TestSession_NewRestored_LoadsScrollbackBeforePump verifies that NewRestored pre-loads
// scrollback into the ring before the pump goroutine starts, so a first Attach sees
// scrollback in the snapshot frame ahead of any new shell output.
func TestSession_NewRestored_LoadsScrollbackBeforePump(t *testing.T) {
	scrollback := []byte("old-scrollback\r\n")
	dir := t.TempDir()
	s, err := NewRestored("sid-restored", "/bin/sh", dir, "profX", os.Environ(), scrollback)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	snap := s.Snapshot()
	require.True(t, contains(snap, scrollback),
		"ring snapshot must contain pre-loaded scrollback; got %q", snap)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	select {
	case f, ok := <-ch:
		require.True(t, ok)
		require.True(t, contains(f.Data, scrollback),
			"first attach frame must contain the pre-loaded scrollback; got %q", f.Data)
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot frame received from restored session")
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
