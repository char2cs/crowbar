package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Deterministic shell synchronisation — no clocks.
//
// These tests drive REAL PTYs and REAL shells, so nearly every wait here answers the
// question "has the shell got there yet?". A duration is not an answer to that question: it
// is a guess about how fast a fork/exec/prompt is on whichever machine happens to be running
// the suite, and when the guess is wrong the test fails for reasons that have nothing to do
// with the code under test.
//
// A shell already has a protocol for announcing where it is — it prints its PROMPT once it
// has finished everything it was asked to do and is blocked reading stdin. The helpers below
// turn that protocol into a test signal, anchored on the session's pump seam
// (PumpNotifyForTest) so a waiter is woken by real progress rather than by a timer.
//
// None of them takes a timeout. A wait that never completes is a HANG, and `go test -timeout`
// is the backstop that reports it — with a goroutine dump, which is strictly more diagnostic
// than a bespoke "did not happen within 5s" message.
// ---------------------------------------------------------------------------

// shellPrompt is the PS1 handed to every test shell. It is deliberately unlikely to occur in
// command output, so finding it on screen unambiguously means "the shell printed its prompt".
const shellPrompt = "<<CRWB-RDY>>"

// testEnv is the environment every test session spawns with: a pinned, prompt-announcing
// shell with no rc file. Without the pin, a session inherits whatever PS1 the developer's
// environment carries and may source dotfiles that speak (or fork children) asynchronously —
// leaving no stable point at which the shell can be said to be ready. Pinning removes that
// nondeterminism at its source instead of trying to out-wait it. exec dedups the environment
// keeping the LAST occurrence of a key, so appending overrides any inherited value.
func testEnv() []string {
	return append(os.Environ(),
		"PS1="+shellPrompt, // the readiness protocol
		"ENV=",             // POSIX sh rc file: none, so no dotfile may speak
		"BASH_ENV=",        // /bin/sh is bash-in-sh-mode on darwin; silence it too
	)
}

// stripANSI removes escape sequences from serializer output, leaving the screen's visible
// text. The serializer emits a clean, ground-state redraw (CSI cursor/SGR, OSC title, then
// literal cell text), so dropping the sequences yields the text a user would see — which is
// what the readiness predicates below match on.
func stripANSI(b []byte) string {
	var out strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c != 0x1b {
			// Keep printable text and newlines; drop other control bytes (notably \r, which
			// the serializer uses for positioning and which would otherwise glue rows).
			if c == '\n' || c >= 0x20 {
				out.WriteByte(c)
			}
			i++
			continue
		}
		i++ // consume ESC
		if i >= len(b) {
			break
		}
		switch b[i] {
		case '[': // CSI: params, then a final byte in @-~
			i++
			for i < len(b) && (b[i] < '@' || b[i] > '~') {
				i++
			}
			i++ // consume the final byte
		case ']', 'P', 'X', '^', '_': // OSC/DCS/SOS/PM/APC: run to BEL or ST (ESC \)
			i++
			for i < len(b) {
				if b[i] == 0x07 {
					i++
					break
				}
				if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		case '(', ')', '*', '+': // charset designation: ESC ( B — two bytes follow
			i += 2
		default: // simple two-byte escape: ESC =, ESC >, ESC M …
			i++
		}
	}
	return out.String()
}

// screenText returns the session's current visible screen as plain text. SerializedForTest is
// deliberately non-consuming, so polling it never perturbs the dirty bit a test may later assert on.
func screenText(s *Session) string { return stripANSI(s.SerializedForTest()) }

// waitScreen blocks until pred is satisfied by the session's screen.
//
// It is the core synchronisation primitive of this package and contains no clock. On each
// iteration it checks the real observable (the screen); if unsatisfied it blocks on the pump
// seam — a signal published only when pumpStep has FULLY processed another chunk (model
// written, frame emitted, dirty set). Waking is proof the screen may have changed, so the loop
// re-reads it.
//
// The check-then-block order is what makes it race-free: the pump seam is 1-buffered, so a
// chunk landing between our check and our receive leaves the edge latched and wakes us
// immediately rather than being missed.
//
// s.Done() is selected on purely so an exited shell fails fast and loudly instead of hanging;
// it is a real signal (the process is gone; no further output can EVER arrive), not a deadline.
func waitScreen(
	t *testing.T,
	s *Session,
	pred func(string) bool,
	what string,
) {
	t.Helper()
	notify := s.PumpNotifyForTest()
	for {
		if pred(screenText(s)) {
			return
		}
		select {
		case <-notify:
			// The pump processed another chunk; loop and re-read the screen.
		case <-s.Done():
			// The shell exited. Re-check once (its final output may have satisfied pred on
			// the way out), then fail — no further output can ever arrive.
			if pred(screenText(s)) {
				return
			}
			t.Fatalf("session exited before %s\nscreen:\n%s", what, screenText(s))
		}
	}
}

// waitPrompt blocks until the shell has printed its prompt — i.e. it has finished starting up,
// has flushed everything it intends to say, and is now blocked reading stdin. It is the honest
// replacement for "sleep until the shell looks settled": nothing follows the prompt until we type.
func waitPrompt(t *testing.T, s *Session) {
	t.Helper()
	waitScreen(t, s, func(scr string) bool {
		return strings.Contains(scr, shellPrompt)
	}, "the shell to print its first prompt")
}

// markerSeq hands out process-unique marker ids so concurrent sessions can never match on
// each other's markers.
var markerSeq atomic.Int64

// runShell writes cmd to the session and blocks until the shell has RUN it and returned to its
// prompt — leaving the session quiescent, with no output still in flight.
//
// The wait is anchored on a unique marker the shell prints after cmd. The marker is emitted
// from two arguments (`printf '%s%s\n' MK7 END`) so the joined token "MK7END" appears ONLY in
// the command's OUTPUT and never in the terminal's echo of the typed line — which contains
// "MK7 END", with a space. Matching the joined form therefore cannot be satisfied by the echo.
//
// Requiring the PROMPT after the marker is the other half: PTY output is ordered, so once the
// prompt that follows the marker is on screen, every byte the command produced has already been
// through the pump. That is what "quiescent" means, established by observation, not by a timer.
func runShell(t *testing.T, s *Session, cmd string) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("MK%dEND", n)
	line := fmt.Sprintf("%s; printf '%%s%%s\\n' MK%d END\n", cmd, n)
	require.NoError(t, s.Write([]byte(line)), "write %q to the shell", cmd)
	waitScreen(t, s, func(scr string) bool {
		i := strings.LastIndex(scr, tok)
		return i >= 0 && strings.Contains(scr[i+len(tok):], shellPrompt)
	}, fmt.Sprintf("command %q to finish and the prompt to return", cmd))
}

// startForeground starts a long-running foreground child and blocks until the KERNEL agrees
// that child owns the terminal — i.e. until the session is genuinely non-idle by the same
// TIOCGPGRP measure IsIdle uses.
//
// The child is a subshell that announces itself and then execs the sleep. That shape is what
// makes the wait exact rather than approximate: job control puts a job in its own process group
// and makes it the terminal's FOREGROUND group BEFORE the job runs, and exec preserves that
// pid/pgroup for the sleep. So the marker's arrival PROVES the foreground process group has
// already flipped away from the shell — no settling, no polling, no "give it 100ms to take
// effect". The assertion below documents that this holds with zero delay; it is checked, not
// assumed.
func startForeground(t *testing.T, s *Session) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("FG%dON", n)
	line := fmt.Sprintf("( printf '%%s%%s\\n' FG%d ON; exec sleep 9999 )\n", n)
	require.NoError(t, s.Write([]byte(line)), "start the foreground child")
	waitScreen(t, s, func(scr string) bool {
		return strings.Contains(scr, tok)
	}, "the foreground child to announce itself")
	require.False(t, s.IsIdle(),
		"the child's first output byte must already prove it owns the terminal "+
			"(job control foregrounds a job's process group before it runs)")
}

// waitForegroundSampled blocks until the pump has run pumpStep's debounced foreground sample at
// least once — i.e. the session has latched the terminal's foreground process group and fired
// whatever OnForegroundReset edge that first sample implies.
//
// It exists for the one test whose model is a FAKE (session_forcesuspend_race_test.go), whose
// screen therefore never shows a shell prompt, so waitPrompt cannot be its readiness signal. The
// sample runs on the FIRST chunk the pump processes (lastFgSampleAt is the zero time, so the
// debounce interval has trivially elapsed) and completes — teardown mutation included — before
// pumpStep publishes its notify edge. Waiting for the stamp is therefore waiting for exactly the
// state the caller depends on, rather than for a duration in which it is assumed to have happened.
func waitForegroundSampled(t *testing.T, s *Session) {
	t.Helper()
	notify := s.PumpNotifyForTest()
	for {
		s.mu.Lock()
		sampled := !s.lastFgSampleAt.IsZero()
		s.mu.Unlock()
		if sampled {
			return
		}
		select {
		case <-notify:
		case <-s.Done():
			t.Fatal("session exited before the pump sampled the foreground process group")
		}
	}
}

// waitFrame blocks until ch delivers a frame. A CLOSED channel is a real signal (the session
// tore down and dropped its clients) and is reported as ok==false. There is no timeout: a frame
// that never arrives is a hang, and `go test -timeout` reports it with the blocked stack.
func waitFrame(
	t *testing.T,
	ch <-chan OutputFrame,
) (OutputFrame, bool) {
	t.Helper()
	f, ok := <-ch
	return f, ok
}

// waitFrameContaining blocks until a frame whose data contains sub arrives, accumulating
// everything seen along the way. A closed channel fails the test — the data can never arrive.
func waitFrameContaining(
	t *testing.T,
	ch <-chan OutputFrame,
	sub string,
) string {
	t.Helper()
	var seen []byte
	for {
		f, ok := <-ch
		if !ok {
			t.Fatalf("channel closed before %q arrived; saw: %q", sub, seen)
		}
		seen = append(seen, f.Data...)
		if containsStr(f.Data, sub) {
			return string(seen)
		}
	}
}

// newTestSession spawns a live session at the default 80×24 size with no scrollback
// override, the shape every session unit test that does not care about size uses. It spawns
// with the pinned, prompt-announcing environment so waitPrompt/runShell can synchronise on the
// shell's own protocol instead of on a clock.
func newTestSession(
	t *testing.T,
	id string,
	dir string,
) (*Session, error) {
	t.Helper()
	return New(id, "/bin/sh", dir, "", testEnv(), 80, 24, 0)
}

func TestSession_NewAndKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-1", dir)
	require.NoError(t, err)
	require.NotNil(t, s)

	s.Kill()

	// Kill's own shutdown closes Done: block on that real signal. A deadline here would
	// only be a second, weaker definition of "too slow".
	<-s.Done()
}

func TestSession_AttachReceivesOutput(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-2", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo hello\n")))

	// Block on the frames themselves until the echoed output shows up; a closed channel
	// (the session died) is the only failure a real signal can report here.
	waitFrameContaining(t, ch, "hello")

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

	// The pump's exit path runs shutdown(), which reaps the child and closes Done. Block on
	// that signal: it fires exactly when the thing under test has happened.
	<-s.Done()

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
	waitFrameContaining(t, ch, "ringmarker")

	s.Detach(ch)

	// Second attach must serialize the current screen, whose grid still shows the marker.
	ch2, err := s.Attach()
	require.NoError(t, err)

	f, ok := waitFrame(t, ch2)
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

	// The overflowing client must have been DROPPED, which closes its channel — that close
	// is the real signal, and fanOutFrameLocked performs it synchronously on the overflowing
	// send. Drain to the close rather than waiting out a deadline: if the drop never happens
	// this ranges forever, and `go test -timeout` reports the hang with the blocked stack.
	for range ch { //nolint:revive // draining to the close IS the assertion
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

				// Drain everything this attach can see, then detach — with no clock. Attach
				// enqueues its serialized redraw SYNCHRONOUSLY (under s.mu, into a buffered
				// channel) and every live fan-out likewise lands synchronously in pumpStep,
				// so "the channel has run dry" is a complete answer to "what did this client
				// receive?" — no window needs waiting out. The old 5ms drain timer added
				// nothing but a chance to truncate a batch and mask a duplicate.
				var received []byte
			drain:
				for {
					select {
					case f, ok := <-ch:
						if !ok {
							break drain
						}
						received = append(received, f.Data...)
					default:
						break drain
					}
				}
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

	// Kill and wait for shutdown — Done() closing IS the shutdown, so block on it.
	s.Kill()
	<-s.Done()

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
	//
	// This negative needs no observation window: Attach serializes and enqueues its redraw
	// SYNCHRONOUSLY, under s.mu, into the buffered channel it returns — so if a frame were
	// ever going to be delivered it is already queued the instant Attach returns. A
	// placeholder has no pump either, so nothing can arrive later. Assert on the channel's
	// length instead of waiting out a guess.
	ch, err := s.Attach()
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Zero(t, len(ch), "placeholder Attach must not deliver a frame")
	s.Detach(ch)

	// Kill must not panic and must close Done().
	s.Kill()
	<-s.Done()
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

	waitIdlePrompt(t, s)
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

	waitIdlePrompt(t, s)
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
	// The shell's own exit closes Done (via the pump's shutdown); ExitCode is recorded in the
	// same shutdown, so Done closing is exactly the signal that makes the assertion below valid.
	<-s.Done()
	assert.Equal(t, 0, s.ExitCode())
}

// TestSession_NewRestored_RebuildsModelFromBlob verifies that NewRestored parses a
// persisted CRWB1 blob, rebuilds the model from its redraw bytes BEFORE the pump starts,
// and a first Attach serializes the restored screen — so prior on-screen content survives
// the round-trip. The blob is produced by a real live session's Snapshot so the header and
// redraw are exactly what the engine persists.
func TestSession_NewRestored_RebuildsModelFromBlob(t *testing.T) {
	dir := t.TempDir()

	// Produce a real persisted blob: a live session that printed a known marker. runShell
	// returns only once the marker's output is through the pump AND the prompt is back, so the
	// screen — and therefore the Snapshot serialized from it — provably carries the marker.
	// No polling for it: the shell's own prompt says when it is there.
	src, err := newTestSession(t, "sid-src", dir)
	require.NoError(t, err)
	waitPrompt(t, src)
	runShell(t, src, "echo restoremarker")
	blob, _ := src.Snapshot()
	src.Kill()
	require.True(t, contains(blob, []byte("restoremarker")),
		"the source screen must carry the marker once the shell is back at its prompt")
	require.True(t, contains(blob, []byte("CRWB1 ")), "blob must carry a CRWB1 header")

	s, err := NewRestored("sid-restored", "/bin/sh", dir, "profX", testEnv(), blob)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "restored session must deliver a serialized attach frame")
	require.True(t, contains(f.Data, []byte("restoremarker")),
		"first attach frame must contain the restored on-screen marker; got %q", f.Data)
}

// waitIdlePrompt blocks until the shell is at its prompt and asserts it is idle THERE.
//
// It replaces the old poll-until-IsIdle helper (and its "did not settle in 5s, skip the test"
// escape hatch, which silently voided the assertions on a slow machine). The prompt is the
// shell's own statement that it has nothing left to run, and at that point it IS the terminal's
// foreground process group — so IsIdle is already true with zero delay. That is asserted here,
// not waited for: if it were ever false at the prompt, polling would only have hidden the bug.
func waitIdlePrompt(t *testing.T, s *Session) {
	t.Helper()
	waitPrompt(t, s)
	require.True(t, s.IsIdle(),
		"a shell sitting at its prompt is the foreground process group, so it must report idle")
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
