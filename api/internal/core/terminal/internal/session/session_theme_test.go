//go:build !windows

package session

import (
	"bytes"
	"image/color"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

type pipeDrainer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	notify chan struct{}
	done   chan struct{}
}

func (d *pipeDrainer) snapshot() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.buf.Bytes()...)
}

func startDrainer(r *os.File) *pipeDrainer {
	d := &pipeDrainer{
		notify: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	go func() {
		defer close(d.done)
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				d.mu.Lock()
				d.buf.Write(buf[:n])
				d.mu.Unlock()
				select {
				case d.notify <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return d
}

func waitContains(t *testing.T, d *pipeDrainer, want string) {
	t.Helper()
	for {
		if bytes.Contains(d.snapshot(), []byte(want)) {
			return
		}
		select {
		case <-d.notify:
		case <-d.done:
			if bytes.Contains(d.snapshot(), []byte(want)) {
				return
			}
			t.Fatalf("PTY closed before %q was written; got %q", want, d.snapshot())
		}
	}
}

func reportCount(d *pipeDrainer) int {
	return bytes.Count(d.snapshot(), []byte("\x1b[?997"))
}

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
		s.Kill()
		_ = pr.Close()
	})
	return s, d
}

func TestSession_SetTheme_NotifiesWhenMode2031Enabled(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-notify")

	s.PumpChunkForTest([]byte("\x1b[?2031h"))
	require.NoError(t, s.SetTheme(color.White, color.Black, true))

	waitContains(t, d, "\x1b[?997;1n")
}

func TestSession_SetTheme_NoNotifyWhenMode2031Disabled(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-nonotify")

	require.NoError(t, s.SetTheme(color.White, color.Black, true))

	s.PumpChunkForTest([]byte("\x1b[?2031h"))
	require.NoError(t, s.SetTheme(color.White, color.Black, true))
	waitContains(t, d, "\x1b[?997;1n")

	require.Equal(t, 1, reportCount(d),
		"exactly one report may exist — the pre-subscription push must have emitted none; got %q",
		d.snapshot())
}

func TestSession_SetTheme_DedupesSamePolarityAndNotifiesOnFlip(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-dedupe")

	s.PumpChunkForTest([]byte("\x1b[?2031h"))

	require.NoError(t, s.SetTheme(color.White, color.Black, true))
	waitContains(t, d, "\x1b[?997;1n")

	require.NoError(t, s.SetTheme(color.White, color.Black, true))
	require.NoError(t, s.SetTheme(color.Black, color.White, false))
	waitContains(t, d, "\x1b[?997;2n")

	require.Equal(t, 2, reportCount(d),
		"want exactly the two flip reports: a same-polarity push must not add a third; got %q",
		d.snapshot())
}

func TestSession_SetTheme_ANotifyWriteFailureIsReturned(t *testing.T) {
	s, _ := newThemeTestSession(t, "sid-theme-notify-fail")
	s.PumpChunkForTest([]byte("\x1b[?2031h"))
	s.Kill()

	require.ErrorContains(t, s.SetTheme(color.White, color.Black, true), "session not live")
}
