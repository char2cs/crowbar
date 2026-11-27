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

const shellPrompt = "<<CRWB-RDY>>"

func testEnv() []string {
	return append(os.Environ(),
		"PS1="+shellPrompt,
		"ENV=",
		"BASH_ENV=",
	)
}

func stripANSI(b []byte) string {
	var out strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c != 0x1b {
			if c == '\n' || c >= 0x20 {
				out.WriteByte(c)
			}
			i++
			continue
		}
		i++
		if i >= len(b) {
			break
		}
		switch b[i] {
		case '[':
			i++
			for i < len(b) && (b[i] < '@' || b[i] > '~') {
				i++
			}
			i++
		case ']', 'P', 'X', '^', '_':
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
		case '(', ')', '*', '+':
			i += 2
		default:
			i++
		}
	}
	return out.String()
}

func screenText(s *Session) string { return stripANSI(s.SerializedForTest()) }

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
		case <-s.Done():
			if pred(screenText(s)) {
				return
			}
			t.Fatalf("session exited before %s\nscreen:\n%s", what, screenText(s))
		}
	}
}

func waitPrompt(t *testing.T, s *Session) {
	t.Helper()
	waitScreen(t, s, func(scr string) bool {
		return strings.Contains(scr, shellPrompt)
	}, "the shell to print its first prompt")
}

var markerSeq atomic.Int64

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

func waitFrame(
	t *testing.T,
	ch <-chan OutputFrame,
) (OutputFrame, bool) {
	t.Helper()
	f, ok := <-ch
	return f, ok
}

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

func newTestSession(
	t *testing.T,
	id string,
	dir string,
) (*Session, error) {
	t.Helper()
	return New(t.Context(), id, "/bin/sh", dir, "", testEnv(), 80, 24, 0)
}

func TestSession_NewAndKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-1", dir)
	require.NoError(t, err)
	require.NotNil(t, s)

	s.Kill()

	<-s.Done()
}

func TestSession_AttachReceivesOutput(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-2", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo hello\n")))

	waitFrameContaining(t, ch, "hello")

	s.Kill()
}

func TestSession_NaturalExitReapsChild(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-natural-exit", dir)
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("exit\n")))

	<-s.Done()

	require.NotNil(t, s.cmd.ProcessState, "natural shell exit must reap the child via cmd.Wait()")
	assert.True(t, s.cmd.ProcessState.Exited(), "child process must have exited")
}

func TestSession_ClientCloseImpliesDone(t *testing.T) {
	s, err := newTestSession(t, "sid-close-order", t.TempDir())
	require.NoError(t, err)
	ch, err := s.Attach()
	require.NoError(t, err)

	doneAtClose := make(chan bool, 1)
	go func() {
		for range ch {
		}
		select {
		case <-s.Done():
			doneAtClose <- true
		default:
			doneAtClose <- false
		}
	}()
	require.NoError(t, s.Write([]byte("exit\n")))
	assert.True(t, <-doneAtClose, "a closed client channel must imply Done is closed")
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

	_ = s.Write([]byte("echo bye\n"))
	s.Kill()
}

func TestSession_ReAttachSerializesScreen(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-5", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo ringmarker\n")))

	waitFrameContaining(t, ch, "ringmarker")

	s.Detach(ch)

	ch2, err := s.Attach()
	require.NoError(t, err)

	f, ok := waitFrame(t, ch2)
	assert.True(t, ok, "expected serialized redraw frame")
	assert.True(t, containsStr(f.Data, "ringmarker"),
		"serialized redraw must contain the on-screen marker, got: %q", f.Data)
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
	s, err := New(t.Context(), "my-id", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	assert.Equal(t, "my-id", s.ID())
	s.Kill()
}

func TestSession_New_BadShell(t *testing.T) {
	dir := t.TempDir()
	_, err := New(t.Context(), "sid-bad", "/nonexistent/shell/binary", dir, "", os.Environ(), 80, 24, 0)
	assert.Error(t, err)
}

func TestSession_WriteAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-7", dir)
	require.NoError(t, err)
	s.Kill()
	<-s.Done()
	writeErr := s.Write([]byte("hello"))
	assert.Error(t, writeErr)
}

func TestSession_ResizeAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-8", dir)
	require.NoError(t, err)
	s.Kill()
	<-s.Done()
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

	for len(ch) > 0 {
		<-ch
	}

	chunk := []byte("x")
	for i := 0; i <= ClientSendBufForTest; i++ {
		s.FanOutForTest(chunk)
	}

	for range ch {
	}
}

func TestIsNormalPTYClose_EOF(t *testing.T) {
	assert.True(t, isNormalPTYClose(io.EOF))
}

func TestIsNormalPTYClose_EIO(t *testing.T) {
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

func TestSession_ReplayLiveHandoff_NoDuplication(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-handoff", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	const rounds = 100

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
					return
				}

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

				for r := 0; r < rounds; r++ {
					if countOccurrences(received, makeChunk(r)) > 1 {
						atomic.AddInt64(&dupCount, 1)
					}
				}
			}
		}()
	}

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

func TestAttachedCount_Transitions(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-lifecycle", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	require.Equal(t, 0, s.AttachedCount())
	ch, err := s.Attach()
	require.NoError(t, err)
	require.Equal(t, 1, s.AttachedCount())
	s.Detach(ch)
	require.Equal(t, 0, s.AttachedCount())

	s.Kill()
	<-s.Done()
	_, err = s.Attach()
	require.Error(t, err, "a dead session refuses new clients")
}

func TestLiveSession_ModelBytesAccountsGrid(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-cap", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	require.Greater(t, s.ModelBytes(), int64(0),
		"a live session must account a positive model footprint")
}

func TestOSC7_CWD_Update(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-osc7", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	chunk := []byte(
		"\x1b]7;file:///old/dir\x07" +
			"\x1b]7;file:///new/dir\x07",
	)
	s.PumpChunkForTest(chunk)

	require.Equal(t, "/new/dir", s.CWD(),
		"CWD must reflect the last OSC 7 path in the chunk")
}

func TestSession_SuspendEligible_WithClients(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-bse1", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	assert.False(t, s.SuspendEligible(false), "with attached clients, must not be eligible")
	assert.False(t, s.SuspendEligible(true), "not even by force")
}

func TestSession_ExitCode_Default(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-ec0", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.Equal(t, -1, s.ExitCode())
}

func TestSession_ExitCode_AfterCleanExit(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-ec1", dir)
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("exit 0\n")))
	<-s.Done()
	assert.Equal(t, 0, s.ExitCode())
}

func TestSession_NewRestored_RebuildsModelFromBlob(t *testing.T) {
	dir := t.TempDir()

	src, err := newTestSession(t, "sid-src", dir)
	require.NoError(t, err)
	waitPrompt(t, src)
	runShell(t, src, "echo restoremarker")
	blob, _ := src.Snapshot()
	src.Kill()
	require.True(t, contains(blob, []byte("restoremarker")),
		"the source screen must carry the marker once the shell is back at its prompt")
	require.True(t, contains(blob, []byte("CRWB1 ")), "blob must carry a CRWB1 header")

	s, err := NewRestored(t.Context(), "sid-restored", "/bin/sh", dir, "profX", testEnv(), blob)
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

func waitIdlePrompt(t *testing.T, s *Session) {
	t.Helper()
	waitPrompt(t, s)
	require.True(t, s.IsIdle(),
		"a shell sitting at its prompt is the foreground process group, so it must report idle")
}

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
			i += len(needle)
		} else {
			i++
		}
	}
	return count
}
