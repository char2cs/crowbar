package model

import (
	"strings"
	"testing"
)

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

func TestStripOSCTitlesLeadingZeroMatchesSingleDigit(t *testing.T) {
	const body = "\xe2\x9c\xb3 ZZLEAK"

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
		if got := string(m.stripOSCTitles([]byte("\x1b]10"))); got != "" {
			t.Fatalf("chunk 1 = %q, want empty (code still ambiguous, withheld)", got)
		}
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
