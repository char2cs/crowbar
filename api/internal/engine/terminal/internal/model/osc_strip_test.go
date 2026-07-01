package model

import (
	"strings"
	"testing"
)

// TestStripOSCTitlesMisprint is the mandated regression for the "phantom Claude Code" grid
// corruption. A cursor-positioned OSC 2 title whose UTF-8 bytes contain 0x9C (the sparkle "✳"
// U+2733 = E2 9C B3, whose middle byte is the C1 String Terminator) must NOT be printed into
// the emulator grid: the pinned x/vt honours that 0x9C as an OSC terminator, drops to ground,
// and stamps the leftover " Claude Code" into its grid. Feeding x/vt the stripped stream
// removes the OSC entirely, so no title text lands in the grid — while escan still captures
// the whole title for the tab. FAILS on the pre-strip code, where the grid contained
// "Claude Code".
func TestStripOSCTitlesMisprint(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b[10;5H\x1b]2;\xe2\x9c\xb3 Claude Code\x07"))

	if snap := gridSnapshot(m); strings.Contains(snap, "Claude Code") {
		t.Fatalf("emulator grid contains mis-printed title text; grid:\n%s", snap)
	}
	if got, want := m.Title(), "✳ Claude Code"; got != want {
		t.Fatalf("Title() = %q, want %q", got, want)
	}
}

// TestStripOSCTitlesLeadingZeroMatchesSingleDigit locks strip and escan to the IDENTICAL numeric
// OSC-code rule for the non-conformant leading-zero title form. A leading-zero OSC 2 whose UTF-8
// body carries 0x9C ("✳" U+2733 = E2 9C B3) must behave exactly like the "\x1b]2;" form: the
// sequence is stripped from the emulator input (no leftover title text mis-printed into the grid
// by x/vt's 0x9C truncation) AND the full title is captured via escan's Atoi("02")=2. Before the
// fix, strip treated the second '0' as "code >= 10, keep" and passed "\x1b]02;" to x/vt (grid
// mis-print) while escan still captured it (double-capture) — strip and capture disagreed.
func TestStripOSCTitlesLeadingZeroMatchesSingleDigit(t *testing.T) {
	const body = "\xe2\x9c\xb3 ZZLEAK" // "✳ ZZLEAK", middle byte 0x9C truncates x/vt's OSC parser

	lz := newVTModel(40, 12, 100)
	lz.Write([]byte("\x1b[10;5H\x1b]02;" + body + "\x07"))

	single := newVTModel(40, 12, 100)
	single.Write([]byte("\x1b[10;5H\x1b]2;" + body + "\x07"))

	if snap := gridSnapshot(lz); strings.Contains(snap, "ZZLEAK") {
		t.Fatalf("leading-zero OSC 2 leaked title text into the grid; grid:\n%s", snap)
	}
	if got, want := lz.Title(), "✳ ZZLEAK"; got != want {
		t.Fatalf("leading-zero Title() = %q, want %q", got, want)
	}
	if got, want := lz.Title(), single.Title(); got != want {
		t.Fatalf("leading-zero Title() = %q, want single-digit Title() = %q", got, want)
	}
}

// TestStripOSCTitlesGridClean tables the icon/title and terminator variants: OSC 0/1/2 with a
// 0x9C, terminated by BEL or the 7-bit ST, must leave the grid empty of title text while the
// title/icon is still captured.
func TestStripOSCTitlesGridClean(t *testing.T) {
	cases := []struct {
		name string
		seq  string
	}{
		{"OSC0 BEL", "\x1b]0;\xe2\x9c\xb3 Claude Code\x07"},
		{"OSC1 BEL", "\x1b]1;\xe2\x9c\xb3 Claude Code\x07"},
		{"OSC2 BEL", "\x1b]2;\xe2\x9c\xb3 Claude Code\x07"},
		{"OSC2 7-bit ST", "\x1b]2;\xe2\x9c\xb3 Claude Code\x1b\\"},
		{"OSC1 7-bit ST", "\x1b]1;\xe2\x9c\xb3 Claude Code\x1b\\"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVTModel(40, 12, 100)
			m.Write([]byte(tc.seq))
			if snap := gridSnapshot(m); strings.Contains(snap, "Claude Code") {
				t.Fatalf("grid contains mis-printed title text; grid:\n%s", snap)
			}
		})
	}
}

// TestStripOSCTitlesSplitAcrossWrites proves a title split across two Write chunks (even
// mid-multibyte, straddling the 0x9C) is fully withheld from x/vt: neither chunk leaks any
// title text into the grid, and the title is still captured whole.
func TestStripOSCTitlesSplitAcrossWrites(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b[3;1H\x1b]2;\xe2\x9c"))
	m.Write([]byte("\xb3 Claude Code\x07"))

	if snap := gridSnapshot(m); strings.Contains(snap, "Claude Code") {
		t.Fatalf("grid contains mis-printed title text after split write; grid:\n%s", snap)
	}
	if got, want := m.Title(), "✳ Claude Code"; got != want {
		t.Fatalf("Title() = %q, want %q", got, want)
	}
}

// TestStripOSCColorsReachEmulator confirms OSC 10/11/12 (default colours, two-digit codes) are
// NOT stripped: they reach x/vt, whose ForegroundColor/BackgroundColor/CursorColor callbacks
// fire and set the shadow colour flags. Stripping only OSC 0/1/2 must leave the colour OSCs
// intact.
func TestStripOSCColorsReachEmulator(t *testing.T) {
	m := newVTModel(40, 12, 100)
	m.Write([]byte("\x1b]10;rgb:1111/2222/3333\x07"))
	m.Write([]byte("\x1b]11;rgb:4444/5555/6666\x1b\\"))
	m.Write([]byte("\x1b]12;rgb:7777/8888/9999\x07"))

	if !m.shadow.fgSet {
		t.Fatalf("OSC 10 (foreground) did not reach x/vt: fgSet=false")
	}
	if !m.shadow.bgSet {
		t.Fatalf("OSC 11 (background) did not reach x/vt: bgSet=false")
	}
	if !m.shadow.cursorColorSet {
		t.Fatalf("OSC 12 (cursor) did not reach x/vt: cursorColorSet=false")
	}
}

// TestStripOSCTitlesOutput drives stripOSCTitles directly and asserts the exact bytes handed
// to the emulator, pinning every framing branch: which OSCs are removed (0/1/2) versus passed
// through (colours, hyperlinks, empty/multi-digit codes), and the surrounding non-OSC bytes.
func TestStripOSCTitlesOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello", "hello"},
		{"OSC2 BEL removed", "\x1b]2;abc\x07", ""},
		{"OSC2 0x9C BEL removed", "\x1b]2;\xe2\x9c\xb3 Claude Code\x07", ""},
		{"OSC2 7-bit ST removed", "\x1b]2;title\x1b\\", ""},
		{"OSC1 removed", "\x1b]1;icon\x07", ""},
		{"OSC0 removed", "\x1b]0;both\x07", ""},
		{"OSC02 leading-zero removed", "\x1b]02;title\x07", ""},
		{"OSC01 leading-zero removed", "\x1b]01;icon\x07", ""},
		{"OSC00 leading-zero removed", "\x1b]00;both\x07", ""},
		{"CUP then OSC2", "\x1b[10;5H\x1b]2;x\x07", "\x1b[10;5H"},
		{"OSC10 fg kept", "\x1b]10;rgb:00/00/00\x07", "\x1b]10;rgb:00/00/00\x07"},
		{"OSC11 bg kept ST", "\x1b]11;#fff\x1b\\", "\x1b]11;#fff\x1b\\"},
		{"OSC12 cursor kept", "\x1b]12;red\x07", "\x1b]12;red\x07"},
		{"OSC4 kept", "\x1b]4;1;red\x07", "\x1b]4;1;red\x07"},
		{"OSC8 hyperlink kept", "\x1b]8;;http://x\x07", "\x1b]8;;http://x\x07"},
		{"OSC133 multi-digit kept", "\x1b]133;A\x07", "\x1b]133;A\x07"},
		{"empty-code OSC kept", "\x1b];foo\x07", "\x1b];foo\x07"},
		{"ESC ESC then char", "\x1b\x1bA", "\x1b\x1bA"},
		{"ESC[ CSI kept", "\x1b[31m", "\x1b[31m"},
		{"OSC2 CAN abort then text", "\x1b]2;ab\x18cd", "cd"},
		{"OSC2 SUB abort then text", "\x1b]2;ab\x1acd", "cd"},
		{"OSC2 ESC-not-ST content removed", "\x1b]2;a\x1bXb\x07", ""},
		{"OSC4 ESC-not-ST kept", "\x1b]4;\x1bXy\x07", "\x1b]4;\x1bXy\x07"},
		{"OSC4 CAN abort kept", "\x1b]4;x\x18yz", "\x1b]4;x\x18yz"},
		{"trailing 0x9C content", "ab\x9ccd", "ab\x9ccd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVTModel(40, 12, 100)
			got := string(m.stripOSCTitles([]byte(tc.in)))
			if got != tc.want {
				t.Fatalf("stripOSCTitles(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestStripOSCTitlesSplitOutput pins the cross-chunk behaviour of stripOSCTitles at the byte
// level: a stripped OSC withholds its introducer until it terminates (never leaking a dangling
// partial to x/vt), while a kept OSC withholds its introducer only until a non-digit byte
// disambiguates the code, then flushes the whole introducer to x/vt.
func TestStripOSCTitlesSplitOutput(t *testing.T) {
	t.Run("stripped title split mid-code and mid-char", func(t *testing.T) {
		m := newVTModel(40, 12, 100)
		if got := string(m.stripOSCTitles([]byte("\x1b]2;\xe2\x9c"))); got != "" {
			t.Fatalf("chunk 1 = %q, want empty (nothing leaked to emulator)", got)
		}
		if got := string(m.stripOSCTitles([]byte("\xb3 x\x07"))); got != "" {
			t.Fatalf("chunk 2 = %q, want empty", got)
		}
	})
	t.Run("kept colour split at code boundary", func(t *testing.T) {
		m := newVTModel(40, 12, 100)
		// Chunk 1 ends on the digits alone: the code is still ambiguous (a further digit could
		// make it 0/1/2 via a leading zero), so the introducer stays withheld — nothing leaks.
		if got := string(m.stripOSCTitles([]byte("\x1b]10"))); got != "" {
			t.Fatalf("chunk 1 = %q, want empty (code still ambiguous, withheld)", got)
		}
		// The ';' is the first non-digit: Atoi("10") = 10 is not a title, so the withheld
		// introducer plus the rest flush through to x/vt.
		if got := string(m.stripOSCTitles([]byte(";rgb:0/0/0\x07"))); got != "\x1b]10;rgb:0/0/0\x07" {
			t.Fatalf("chunk 2 = %q, want %q", got, "\x1b]10;rgb:0/0/0\x07")
		}
	})
	t.Run("stripped title code withheld across chunk", func(t *testing.T) {
		m := newVTModel(40, 12, 100)
		if got := string(m.stripOSCTitles([]byte("\x1b]2"))); got != "" {
			t.Fatalf("chunk 1 = %q, want empty (single-digit code still ambiguous, withheld)", got)
		}
		if got := string(m.stripOSCTitles([]byte(";title\x07"))); got != "" {
			t.Fatalf("chunk 2 = %q, want empty", got)
		}
	})
}
