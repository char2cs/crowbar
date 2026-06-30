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
		{"osc st 8bit", "\x1b]0;t\x9c", ""},
		{"osc esc backslash", "\x1b]0;t\x1b\\", ""},
		{"osc esc non-backslash stays", "\x1b]0;\x1bX", "\x1b]0;\x1bX"},
		{"osc can", "\x1b]0;\x18", ""},
		{"dcs incomplete", "\x1bP1$r", "\x1bP1$r"},
		{"dcs st terminated", "\x1bP1\x1b\\", ""},
		{"dcs 8bit st", "\x1bP1\x9c", ""},
		{"dcs can", "\x1bP1\x18", ""},
		{"dcs esc non-backslash stays", "\x1bP1\x1bX", "\x1bP1\x1bX"},
		{"c1 csi 8bit incomplete", "\x9b31", "\x9b31"},
		{"c1 osc 8bit incomplete", "\x9d0;t", "\x9d0;t"},
		{"c1 dcs 8bit incomplete", "\x901", "\x901"},
		{"c1 sos 8bit incomplete", "\x98x", "\x98x"},
		{"c1 pm 8bit incomplete", "\x9ex", "\x9ex"},
		{"c1 apc 8bit incomplete", "\x9fx", "\x9fx"},
		{"sequence then ground resets start", "\x1b[0m\x1b[3", "\x1b[3"},
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
