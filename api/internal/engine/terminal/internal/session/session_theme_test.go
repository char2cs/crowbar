//go:build !windows

package session

import (
	"bytes"
	"image/color"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// pipeDrainer continuously reads a fake-ptmx pipe read-end into a buffer so a test can
// observe (and assert the ABSENCE of) bytes the session wrote to the PTY, without the
// stale-blocked-Read races a per-assertion reader goroutine would create.
type pipeDrainer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (d *pipeDrainer) snapshot() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.buf.Bytes()...)
}

func startDrainer(r *os.File) *pipeDrainer {
	d := &pipeDrainer{}
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				d.mu.Lock()
				d.buf.Write(buf[:n])
				d.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return d
}

// waitContains polls the drained bytes until they contain want, or fails on timeout.
func waitContains(t *testing.T, d *pipeDrainer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(d.snapshot(), []byte(want)) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q; got %q", want, d.snapshot())
}

// newThemeTestSession wires a model-driven session onto a fake ptmx (os.Pipe write end)
// and returns it plus a drainer over the read end. Mirrors the harness in
// session_responsesink_deadlock_test.go.
func newThemeTestSession(t *testing.T, id string) (*Session, *pipeDrainer) {
	t.Helper()
	s := newBareSession(id, "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	s.ptmx = pw
	s.startResponseSink(s.ptmx)

	d := startDrainer(pr)
	t.Cleanup(func() {
		s.Kill() // closes pw -> drainer sees EOF and exits; releases the reply writer
		_ = pr.Close()
	})
	return s, d
}

// TestSession_SetTheme_NotifiesWhenMode2031Enabled is Gap B: with the app subscribed
// (DEC 2031), a theme switch must inject the CSI ?997;n theme-change report into the PTY so a
// running Claude Code re-queries and follows the theme live.
func TestSession_SetTheme_NotifiesWhenMode2031Enabled(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-notify")

	s.PumpChunkForTest([]byte("\x1b[?2031h"))  // app subscribes
	s.SetTheme(color.White, color.Black, true) // switch to dark

	waitContains(t, d, "\x1b[?997;1n") // dark report
}

// TestSession_SetTheme_NoNotifyWhenMode2031Disabled: a shell (or any app that never enabled
// 2031) must NOT receive a CSI ?997;n report — that would corrupt its input line.
func TestSession_SetTheme_NoNotifyWhenMode2031Disabled(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-nonotify")

	s.SetTheme(color.White, color.Black, true) // no ?2031h beforehand

	time.Sleep(250 * time.Millisecond)
	if bytes.Contains(d.snapshot(), []byte("\x1b[?997")) {
		t.Fatalf("emitted a theme-notify report without a 2031 subscription: %q", d.snapshot())
	}
}

// TestSession_SetTheme_DedupesSamePolarityAndNotifiesOnFlip: only the FIRST push of a given
// polarity (and every subsequent flip) emits a report — an unrelated same-polarity theme
// tweak must not spam a running app with redundant re-query triggers.
func TestSession_SetTheme_DedupesSamePolarityAndNotifiesOnFlip(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-dedupe")

	s.PumpChunkForTest([]byte("\x1b[?2031h"))

	s.SetTheme(color.White, color.Black, true) // dark -> notify
	waitContains(t, d, "\x1b[?997;1n")

	s.SetTheme(color.White, color.Black, true) // dark again -> deduped
	time.Sleep(200 * time.Millisecond)
	if n := bytes.Count(d.snapshot(), []byte("\x1b[?997")); n != 1 {
		t.Fatalf("same-polarity push not deduped: %d reports, want 1 (%q)", n, d.snapshot())
	}

	s.SetTheme(color.Black, color.White, false) // light -> notify
	waitContains(t, d, "\x1b[?997;2n")
	if n := bytes.Count(d.snapshot(), []byte("\x1b[?997")); n != 2 {
		t.Fatalf("polarity flip not notified: %d reports, want 2 (%q)", n, d.snapshot())
	}
}
