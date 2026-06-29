package session

import (
	"bytes"
	"testing"
)

func TestDecModeTracker(t *testing.T) {
	t.Run("tracks set/reset of net-active modes", func(t *testing.T) {
		tr := newDecModeTracker()
		tr.observe([]byte("\x1b[?1h\x1b[?1000h\x1b[?1006h"))         // app cursor keys + mouse + SGR on
		tr.observe([]byte("some output \x1b[31mred\x1b[0m more"))   // no DEC private modes
		tr.observe([]byte("\x1b[?1000l"))                            // mouse off again
		got := tr.preamble()
		// 1 and 1006 still on, 1000 turned back off → omitted
		if want := []byte("\x1b[?1h\x1b[?1006h"); !bytes.Equal(got, want) {
			t.Fatalf("preamble = %q, want %q", got, want)
		}
	})

	t.Run("multi-param DECSET", func(t *testing.T) {
		tr := newDecModeTracker()
		tr.observe([]byte("\x1b[?1049;1000;1006h"))
		got := tr.preamble()
		if want := []byte("\x1b[?1000h\x1b[?1006h\x1b[?1049h"); !bytes.Equal(got, want) {
			t.Fatalf("preamble = %q, want %q", got, want)
		}
	})

	t.Run("clean exit (set then reset) yields no preamble", func(t *testing.T) {
		tr := newDecModeTracker()
		tr.observe([]byte("\x1b[?1h\x1b[?1000h\x1b[?2004h")) // app enabled modes
		tr.observe([]byte("\x1b[?2004l\x1b[?1000l\x1b[?1l")) // app cleaned up on exit
		if got := tr.preamble(); got != nil {
			t.Fatalf("preamble = %q, want nil after clean reset", got)
		}
	})

	t.Run("ignores untracked modes and queries", func(t *testing.T) {
		tr := newDecModeTracker()
		tr.observe([]byte("\x1b[?2026h\x1b[?12h\x1b[?25l")) // sync update, cursor blink, cursor hide — untracked
		tr.observe([]byte("\x1b[?1;2c\x1b[6n"))             // DA reply + DSR — not DECSET, must not match
		if got := tr.preamble(); got != nil {
			t.Fatalf("preamble = %q, want nil (no tracked modes)", got)
		}
	})

	t.Run("sequence split across chunk boundary", func(t *testing.T) {
		tr := newDecModeTracker()
		tr.observe([]byte("output\x1b[?100")) // split mid-sequence
		tr.observe([]byte("0h rest"))          // completes ?1000h
		if got := tr.preamble(); !bytes.Equal(got, []byte("\x1b[?1000h")) {
			t.Fatalf("preamble = %q, want \\x1b[?1000h across boundary", got)
		}
	})
}
