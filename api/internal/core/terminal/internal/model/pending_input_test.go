package model

import "testing"

func TestScanPartial(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ground only", "abc", ""},
		{"esc 2-byte complete", "\x1bM", ""},
		{"esc abort can", "\x1b\x18", ""},
		{"esc default abort", "\x1b\x7f", ""},
		{"esc intermediate incomplete", "\x1b(", "\x1b("},
		{"esc intermediate two then final", "\x1b(!B", ""},
		{"esc intermediate incomplete two", "\x1b(!", "\x1b(!"},
		{"csi incomplete", "ok\x1b[31", "\x1b[31"},
		{"csi complete", "\x1b[31m", ""},
		{"csi abort can", "\x1b[3\x18", ""},
		{"osc incomplete", "\x1b]0;title", "\x1b]0;title"},
		{"osc bel terminated", "\x1b]0;t\x07", ""},
		{"osc esc backslash", "\x1b]0;t\x1b\\", ""},
		{"osc esc non-backslash stays", "\x1b]0;\x1bX", "\x1b]0;\x1bX"},
		{"osc can", "\x1b]0;\x18", ""},
		{"dcs incomplete", "\x1bP1$r", "\x1bP1$r"},
		{"dcs st terminated", "\x1bP1\x1b\\", ""},
		{"dcs can", "\x1bP1\x18", ""},
		{"dcs esc non-backslash stays", "\x1bP1\x1bX", "\x1bP1\x1bX"},
		{"sequence then ground resets start", "\x1b[0m\x1b[3", "\x1b[3"},

		// UTF-8 transparency: bytes 0x80-0x9F are UTF-8 lead/continuation bytes in this
		// stream, NEVER 8-bit C1 introducers. Standalone, they are inert ground content, so
		// the scanner must NOT flip into a CSI/OSC/string state on them (the re-attach
		// corruption defect). All of these end in ground -> nil.
		{"c1 csi byte is ground", "\x9b31", ""},
		{"c1 osc byte is ground", "\x9d0;t", ""},
		{"c1 dcs byte is ground", "\x901", ""},
		{"c1 sos byte is ground", "\x98x", ""},
		{"c1 pm byte is ground", "\x9ex", ""},
		{"c1 apc byte is ground", "\x9fx", ""},
		// The exact glyph from the ground-truth blob: "▘" = U+2598 = E2 96 98. Its third
		// byte 0x98 is the C1 SOS code point; it must stay ground, not swallow the frame.
		{"box-drawing glyph stays ground", "box ▘ ▝▝ done", ""},
		{"glyph then real incomplete csi", "▘▝\x1b[54", "\x1b[54"},
		// 8-bit ST (0x9C) inside a string is a UTF-8 byte, not a terminator: the string
		// stays open, so the trailing tail is the whole ESC-led sequence (not nil).
		{"osc 8bit st not honoured", "\x1b]0;t\x9c", "\x1b]0;t\x9c"},
		{"dcs 8bit st not honoured", "\x1bP1\x9c", "\x1bP1\x9c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(scanPartial([]byte(tc.in)))
			if got != tc.want {
				t.Fatalf("scanPartial(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
