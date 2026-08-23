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

// pipeDrainer continuously reads a fake-ptmx pipe read-end into a buffer so a test can
// observe (and assert the ABSENCE of) bytes the session wrote to the PTY, without the
// stale-blocked-Read races a per-assertion reader goroutine would create.
//
// It publishes a real progress signal rather than making callers poll it: `notify` is a
// 1-buffered coalescing edge ("more bytes landed since you last looked") and `done` closes when
// the pipe reaches EOF, at which point no further byte can EVER arrive. Together they are the
// two things a waiter needs, and neither is a clock.
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

// startDrainer is the ONLY constructor: both channels must exist before the reader goroutine
// (or any waiter) touches them — a nil channel receive blocks forever.
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
				case d.notify <- struct{}{}: // coalescing edge; never blocks the reader
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

// waitContains blocks until the bytes the session wrote to the PTY contain want.
//
// Check-then-block against the 1-buffered notify edge, so bytes landing between the check and
// the receive leave the edge latched rather than being missed. EOF (done) is a real signal, not
// a deadline: once the pipe is closed no further byte can arrive, so a re-check and a failure
// there is a statement of fact, not an expiry.
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

// reportCount counts theme-notify reports (CSI ?997;n) written to the PTY so far.
func reportCount(d *pipeDrainer) int {
	return bytes.Count(d.snapshot(), []byte("\x1b[?997"))
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
//
// A negative fenced by a duration ("nothing showed up in 250ms") proves nothing: it asserts the
// report is SLOW, not that it is absent. This uses a SEQUENCE BARRIER instead. After the
// unsubscribed push, the app subscribes and the SAME SetTheme call is made again — and that one
// DOES write a report, down the identical SetTheme → s.Write → ptmx path. PTY writes are FIFO
// and both pushes issue theirs synchronously inside SetTheme, so had the unsubscribed push
// written anything it would necessarily sit AHEAD of the subscribed one in the pipe. Once the
// subscribed report is observed, the drained bytes are therefore COMPLETE with respect to both
// pushes, and counting reports is an exact, closed question: exactly one means the unsubscribed
// push wrote nothing at all.
func TestSession_SetTheme_NoNotifyWhenMode2031Disabled(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-nonotify")

	s.SetTheme(color.White, color.Black, true) // no ?2031h beforehand: must not notify

	s.PumpChunkForTest([]byte("\x1b[?2031h"))  // the app subscribes…
	s.SetTheme(color.White, color.Black, true) // …so this identical push DOES notify
	waitContains(t, d, "\x1b[?997;1n")

	require.Equal(t, 1, reportCount(d),
		"exactly one report may exist — the pre-subscription push must have emitted none; got %q",
		d.snapshot())
}

// TestSession_SetTheme_DedupesSamePolarityAndNotifiesOnFlip: only the FIRST push of a given
// polarity (and every subsequent flip) emits a report — an unrelated same-polarity theme
// tweak must not spam a running app with redundant re-query triggers.
//
// The dedupe (a negative) is again proved by a sequence barrier rather than by an observation
// window: the polarity FLIP that follows it is itself a report-writing push down the same path,
// so once the flip's report is on the wire, any report the deduped push might have written is
// already on the wire too. Counting at that point settles both claims at once — 2 reports means
// the flip notified AND the same-polarity push did not.
func TestSession_SetTheme_DedupesSamePolarityAndNotifiesOnFlip(t *testing.T) {
	s, d := newThemeTestSession(t, "sid-theme-dedupe")

	s.PumpChunkForTest([]byte("\x1b[?2031h"))

	s.SetTheme(color.White, color.Black, true) // dark -> notify
	waitContains(t, d, "\x1b[?997;1n")

	s.SetTheme(color.White, color.Black, true)  // dark again -> must be deduped
	s.SetTheme(color.Black, color.White, false) // light -> notify (the barrier)
	waitContains(t, d, "\x1b[?997;2n")

	require.Equal(t, 2, reportCount(d),
		"want exactly the two flip reports: a same-polarity push must not add a third; got %q",
		d.snapshot())
}
