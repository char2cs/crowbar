package session

import (
	"bytes"
	"testing"
)

func TestSanitizeReplaySnapshot(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Stripped: queries/responses that re-trigger echo on replay.
		{"DA response", "hello\x1b[?1;2cworld", "helloworld"},
		{"DA request", "a\x1b[cb", "ab"},
		{"secondary DA", "a\x1b[>0;276;0cb", "ab"},
		{"DSR cursor", "x\x1b[6ny", "xy"},
		{"OSC11 color query", "p\x1b]11;?\x07q", "pq"},
		{"OSC11 color query ST", "p\x1b]11;?\x1b\\q", "pq"},
		{"OSC4 palette query", "p\x1b]4;1;?\x07q", "pq"},
		// Stripped: set-title (avoids stale/garbled tab on replay).
		{"OSC set-title BEL", "\x1b]2;my title\x07rest", "rest"},
		{"OSC set-title ST", "\x1b]0;name\x1b\\rest", "rest"},
		// Stripped: app-only private modes that must not persist to a shell.
		{"focus mode set", "\x1b[?1004hZ", "Z"},
		{"mouse modes", "\x1b[?1000h\x1b[?1006hM", "M"},
		{"bracketed paste", "\x1b[?2004hN", "N"},
		{"alt screen", "\x1b[?1049hA", "A"},
		// Preserved: everything that draws the visual screen.
		{"SGR color", "\x1b[01;32mgreen\x1b[0m", "\x1b[01;32mgreen\x1b[0m"},
		{"cursor move", "\x1b[10;5Hx", "\x1b[10;5Hx"},
		{"erase", "\x1b[2Jx", "\x1b[2Jx"},
		{"box drawing utf8", "─│╭╮", "─│╭╮"},
		{"plain text", "just text", "just text"},
		// A realistic mixed line: keep colors+text, drop the trailing DA reply.
		{"mixed", "\x1b[32mok\x1b[0m\x1b[?1;2c", "\x1b[32mok\x1b[0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeReplaySnapshot([]byte(tc.in))
			if !bytes.Equal(got, []byte(tc.want)) {
				t.Fatalf("sanitizeReplaySnapshot(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
