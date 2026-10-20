package model

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
)

// wrapFlags renders a model's wrap bits as text: the grid rows, then the scrollback lines.
func wrapFlags(
	t *testing.T,
	m TerminalModel,
) string {
	t.Helper()
	vm := asVT(t, m)
	var b strings.Builder
	b.WriteString("grid")
	for y := 0; y < vm.emu.Height(); y++ {
		b.WriteString(" " + flagLetter(vm.emu.RowWrapped(y)))
	}
	b.WriteString(" | scrollback")
	for i := 0; i < vm.emu.ScrollbackLen(); i++ {
		b.WriteString(" " + flagLetter(vm.emu.ScrollbackRowWrapped(i)))
	}
	return b.String()
}

// A keyframe written into a fresh terminal rebuilds every wrap bit and every cell, on the grid
// and across the scrollback boundary.
func TestSerialize_RoundTripPreservesWrapFlags(t *testing.T) {
	const cols, rows = 20, 5
	url := "see " + wrapTestURL // 62 cells: a 4-row chain at 20 columns
	hardBreak := "see " + wrapTestURL[:16] + "\r\n" + wrapTestURL[16:]
	chain3 := strings.Repeat("0123456789", 5)
	// x/vt trims the printed space off the stored head, so only padding re-joins the line.
	spaceHead := "0123456789012345678 abc"
	blankCont := strings.Repeat("x", 20) + " " // the space alone wraps onto the next row
	// Four history lines put the head on the client's bottom row, so the wrap into the red cell
	// scrolls there and fills the new row with the pen's background; the model wrapped in place.
	redCont := "1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7\r\n8\r\n9\x1b[H" + strings.Repeat("x", 20) + "\x1b[41mAB\x1b[m"
	fullCont := strings.Repeat("x", 20) + strings.Repeat("y", 20) // the continuation ends at the last column
	wideCont := strings.Repeat("x", 20) + strings.Repeat("y", 18) + "日"
	tests := []struct {
		name, bytes string
		flags       string
	}{
		{name: "a soft-wrapped URL", bytes: url, flags: "grid F T T T F | scrollback"},
		{name: "an exact-fit hard break", bytes: hardBreak, flags: "grid F F T T F | scrollback"},
		{name: "a 3-row chain", bytes: chain3, flags: "grid F T T F F | scrollback"},
		{name: "a head in the scrollback, its tail on screen", bytes: url + "\r\n\r\n", flags: "grid T T T F F | scrollback F"},
		{name: "a head ending in a printed space", bytes: spaceHead + strings.Repeat("\r\n", 5), flags: "grid F F F F F | scrollback F T"},
		{name: "a blank continuation", bytes: blankCont + "\r\nnext", flags: "grid F T F F F | scrollback"},
		{name: "a styled URL", bytes: "\x1b[4;34m" + url + "\x1b[m", flags: "grid F T T T F | scrollback"},
		{name: "the alternate screen", bytes: "\x1b[?1049h" + url, flags: "grid F T T T F | scrollback"},
		{name: "a coloured continuation the client reaches by a scroll", bytes: redCont, flags: "grid F T F F F | scrollback F F F F"},
		{name: "a continuation that fills its row", bytes: fullCont + "\r\nz", flags: "grid F T F F F | scrollback"},
		{name: "a continuation ending in a wide glyph at the last column", bytes: wideCont + "\r\nz", flags: "grid F T F F F | scrollback"},
		{
			name:  "chains deep in the scrollback",
			bytes: strings.Join([]string{url, hardBreak, chain3, blankCont, spaceHead, url}, "\r\n"),
			flags: "grid T F T T T | scrollback F T T T F F T T F T T F T F",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, s := New(cols, rows, 100)
			t.Cleanup(m.Close)
			m.Write([]byte(tc.bytes))
			require.Equal(t, tc.flags, wrapFlags(t, m), "test setup: the model's wrap bits")

			fresh, _ := New(cols, rows, 100)
			t.Cleanup(fresh.Close)
			fresh.Write(s.Serialize(m))

			require.Equal(t, wrapFlags(t, m), wrapFlags(t, fresh), "wrap bits")
			require.Equal(t, gridString(m), gridString(fresh), "grid")
			require.Equal(t, scrollbackStrings(m), scrollbackStrings(fresh), "scrollback")
			assertSameCells(t, m, fresh)
		})
	}
}

func TestPadToCols(t *testing.T) {
	ab := []uv.Cell{{Content: "a", Width: 1}, {Content: "b", Width: 1}}
	tests := []struct {
		name  string
		cells []uv.Cell
		cols  int
		want  string
	}{
		{name: "a trimmed line is padded with blanks", cells: ab, cols: 4, want: "ab  "},
		{name: "an empty line becomes a blank row", cells: nil, cols: 3, want: "   "},
		{name: "a full line is kept", cells: ab, cols: 2, want: "ab"},
		{name: "a line wider than cols is kept", cells: ab, cols: 1, want: "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := padToCols(tc.cells, tc.cols)
			require.Equal(t, tc.want, plainText(got))
			for _, c := range got[len(tc.cells):] {
				require.True(t, c.Equal(&uv.EmptyCell), "padding is the empty cell")
			}
		})
	}

	t.Run("never writes into the line's spare capacity", func(t *testing.T) {
		backing := []uv.Cell{{Content: "a", Width: 1}, {Content: "z", Width: 1}}
		line := backing[:1]
		require.Equal(t, "a ", plainText(padToCols(line, 2)))
		require.Equal(t, "z", backing[1].Content, "the emulator's stored line is untouched")
	})
}

// TestAppendChainRow pins the one row rule every writer shares: a head runs to the last column
// with no break; a continuation keeps a cell and erases what it leaves short; the rest is trimmed.
func TestAppendChainRow(t *testing.T) {
	ab := []uv.Cell{{Content: "a", Width: 1}, {Content: "b", Width: 1}}
	blank := []uv.Cell{uv.EmptyCell, uv.EmptyCell}
	wide := []uv.Cell{{Content: "a", Width: 1}, {Content: "b", Width: 1}, {Content: "日", Width: 2}, {}}
	tests := []struct {
		name                 string
		cells                []uv.Cell
		wrapped, nextWrapped bool
		want                 string
	}{
		{name: "a head is padded and unbroken", cells: ab, nextWrapped: true, want: "ab  "},
		{name: "a blank head is padded", cells: nil, nextWrapped: true, want: "    "},
		{name: "a row is trimmed and broken", cells: append(ab, blank...), want: "ab|"},
		{name: "a blank row is only the break", cells: blank, want: "|"},
		{name: "a short continuation erases the rest", cells: append(ab, blank...), wrapped: true, want: "ab\x1b[K|"},
		{name: "a blank continuation keeps one cell", cells: blank, wrapped: true, want: " \x1b[K|"},
		{name: "a continuation at the last column erases nothing", cells: append(ab, ab...), wrapped: true, want: "abab|"},
		{name: "a wide glyph that reaches the last column erases nothing", cells: wide, wrapped: true, want: "ab日|"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, string(appendChainRow(nil, tc.cells, 4, tc.wrapped, tc.nextWrapped, "|")))
		})
	}

	t.Run("appends to what dst holds", func(t *testing.T) {
		require.Equal(t, "\x1b[1;1Hab", string(appendChainRow([]byte("\x1b[1;1H"), ab, 4, false, false, "")))
	})
}

func TestChainEnd(t *testing.T) {
	tests := []struct {
		name, bytes string
		y, want     int
	}{
		{name: "a plain row ends its own chain", bytes: "a\r\nb", y: 0, want: 0},
		{name: "a head runs to its last continuation", bytes: strings.Repeat("x", 50), y: 0, want: 2},
		{name: "a chain may start at a continuation", bytes: strings.Repeat("x", 50), y: 1, want: 2},
		{name: "a hard break ends a chain", bytes: strings.Repeat("x", 25) + "\r\n" + strings.Repeat("y", 25), y: 0, want: 1},
		{name: "the chain after a hard break", bytes: strings.Repeat("x", 25) + "\r\n" + strings.Repeat("y", 25), y: 2, want: 3},
		{name: "a chain stops at the last row", bytes: "\r\n\r\n\r\n\r\n" + strings.Repeat("x", 25), y: 4, want: 5},
		{name: "the top row continuing the scrollback", bytes: strings.Repeat("x", 140), y: 0, want: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := New(20, 6, 100)
			t.Cleanup(m.Close)
			m.Write([]byte(tc.bytes))
			require.Equal(t, tc.want, chainEnd(asVT(t, m).emu, tc.y, 6))
		})
	}
}

// assertSameCells compares content and style cell by cell, grid then scrollback. A scrollback
// line reports as a row above the grid: -1 is the newest.
func assertSameCells(
	t *testing.T,
	want TerminalModel,
	got TerminalModel,
) {
	t.Helper()
	w, g := asVT(t, want).emu, asVT(t, got).emu
	for y := 0; y < w.Height(); y++ {
		for x := 0; x < w.Width(); x++ {
			assertCellEqual(t, x, y, w.CellAt(x, y), g.CellAt(x, y))
		}
	}
	n := w.ScrollbackLen()
	for i := 0; i < n; i++ {
		for x := 0; x < w.Width(); x++ {
			assertCellEqual(t, x, i-n, lineCellAt(w.ScrollbackLine(i), x), lineCellAt(g.ScrollbackLine(i), x))
		}
	}
}

func asVT(
	t *testing.T,
	m TerminalModel,
) *vtModel {
	t.Helper()
	vm, ok := m.(*vtModel)
	require.True(t, ok, "%T is not the x/vt model", m)
	return vm
}
