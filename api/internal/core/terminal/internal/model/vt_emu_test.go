package model

import (
	"cmp"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRowWrapped pins the emulator's per-row wrap bit to xterm's isWrapped: autowrap sets
// it, the row carries it into the scrollback, and it drops only where xterm drops it.
func TestRowWrapped(t *testing.T) {
	const (
		wrap   = "0123456789ABCDEFGHIJ" // row 0 full, autowraps into row 1
		thirty = "0123456789ABCDEFGHIJKLMNOPQRST"
		// One history line and "top" on row 0, for the rows that park the cursor below the grid.
		parked = "hist\r\n\r\n\r\n\r\n\x1b[Htop"
	)
	crlf := func(n int) string { return strings.Repeat("\r\n", n) }
	tests := []struct {
		name       string
		bytes      string
		resize     [2]int // cols, rows; zero means no resize
		after      string // written after the resize
		maxLines   int    // scrollback cap; zero means 100
		grid       string
		scrollback string // empty means not checked
		screen     string // ScreenText; empty means not checked
		history    string // scrollback text joined by "|"; empty means not checked
	}{
		{name: "autowrap sets it", bytes: wrap, grid: "F T F F"},
		{name: "exact fit + CRLF is a hard break", bytes: "0123456789\r\nABCDEFGHIJ", grid: "F F F F"},
		{name: "autowrap off", bytes: "\x1b[?7l" + wrap, grid: "F F F F"},
		{name: "EL0 at col 0 clears", bytes: wrap + "\x1b[2;1H\x1b[K", grid: "F F F F"},
		{name: "EL0 mid-row keeps", bytes: wrap + "\x1b[2;5H\x1b[K", grid: "F T F F"},
		{name: "ED2 clears", bytes: wrap + "\x1b[2J", grid: "F F F F"},
		{
			name:  "flag travels with its row; orphan keeps it when scrollback holds the head",
			bytes: wrap + "\r\n\r\n\r\nX", grid: "T F F F", scrollback: "F",
		},
		{name: "scrollback carries flags", bytes: thirty + crlf(4), grid: "F F F F", scrollback: "F T T"},
		{name: "alt screen never keeps an orphan row 0", bytes: "\x1b[?1049h" + wrap + crlf(4), grid: "F F F F"},
		{name: "alt screen drops the flag of a row scrolled to the top", bytes: "\x1b[?1049h" + wrap + crlf(3), grid: "F F F F"},
		{
			name:  "a wider screen rejoins a chain",
			bytes: thirty + crlf(3), resize: [2]int{15, 4}, grid: "T F F F", scrollback: "F", history: "0123456789ABCDE",
		},
		{
			name:  "a narrower screen rewraps a chain",
			bytes: thirty + crlf(3), resize: [2]int{6, 4},
			grid: "T F F F", scrollback: "F T T T", history: "012345|6789AB|CDEFGH|IJKLMN",
		},
		{
			name:  "a taller screen pulls history down",
			bytes: thirty + crlf(3), resize: [2]int{10, 5}, grid: "T T F F F", scrollback: "F", history: "0123456789",
		},
		{name: "a wide glyph past the last column wraps early", bytes: "012345678日x", grid: "F T F F", screen: "012345678\n日x\n\n"},
		{name: "a wide glyph ending at the last column leaves a pending wrap", bytes: "01234567日x", grid: "F T F F", screen: "01234567日\nx\n\n"},
		{name: "EL1 at the last column keeps", bytes: wrap + "\x1b[2;10H\x1b[1K", grid: "F T F F"},
		{name: "ECH with n >= cols keeps", bytes: wrap + "\x1b[2;1H\x1b[10X", grid: "F T F F"},
		{
			name:  "ring eviction clears the new oldest line",
			bytes: thirty + crlf(5), maxLines: 3, grid: "F F F F", scrollback: "F T F",
		},
		{name: "ED0 at col 0 clears the cursor row and below", bytes: thirty + "\x1b[2;1H\x1b[J", grid: "F F F F"},
		{name: "ED0 mid-row keeps the cursor row, clears below", bytes: thirty + "\x1b[2;5H\x1b[J", grid: "F T F F"},
		{name: "ED1 clears the cursor row and above", bytes: thirty + "\x1b[2;5H\x1b[1J", grid: "F F T F"},
		{name: "ED1 at the last column clears the row below", bytes: thirty + "\x1b[2;10H\x1b[1J", grid: "F F F F"},
		{name: "ED2 saves rows with their flags", bytes: wrap + "\x1b[2J", grid: "F F F F", scrollback: "F T"},
		{
			name:  "ED2 drops the flag of a row whose blank head is not saved",
			bytes: "X" + crlf(4) + "\x1b[H" + wrap + "\x1b[1;1H\x1b[2K\x1b[2J", grid: "F F F F", scrollback: "F F",
		},
		{name: "LF onto a wrapped row clears it", bytes: wrap + "\x1b[1;1H\n", grid: "F F F F"},
		{name: "IND onto a wrapped row keeps it", bytes: wrap + "\x1b[1;1H\x1bD", grid: "F T F F"},
		{name: "IL moves flags down with their rows", bytes: wrap + "\x1b[2;1H\x1b[L", grid: "F F T F"},
		{name: "DL moves flags up with their rows", bytes: thirty + "\x1b[2;1H\x1b[M", grid: "F T F F"},
		{name: "a scroll adds an unwrapped bottom row", bytes: thirty + "UVWXYZabcd\r\n", grid: "T T T F", scrollback: "F"},
		{
			name:  "autowrap on the bottom row scrolls its head into the scrollback",
			bytes: thirty + "UVWXYZabcdefghijklmn", grid: "T T T T", scrollback: "F",
		},
		{
			name:  "a margin-confined scroll saves its top row without the head left on screen",
			bytes: "X" + crlf(4) + "\x1b[2;4r" + wrap + "\x1b[S", grid: "F F F F", scrollback: "F F",
		},
		{name: "RI moves flags down with their rows", bytes: wrap + "\x1b[H\x1bM", grid: "F F T F"},
		{name: "RIS clears", bytes: wrap + "\x1bc", grid: "F F F F"},
		{name: "a 1-row alt screen never flags row 0", bytes: "\x1b[?1049h", resize: [2]int{10, 1}, after: wrap, grid: "F"},
		{
			name:  "a scroll under a top margin drops row 0's flag",
			bytes: thirty + crlf(3) + "\x1b[2;4r\x1b[S", grid: "F F F F", scrollback: "F T F",
		},
		// A shrink with the cursor on the last row scrolls the top rows into the scrollback, as in
		// xterm; a flag access at the cursor afterwards must not panic.
		{
			name:  "alt exit after a shrink, then ED0",
			bytes: parked + "\x1b[4;1Hprompt\x1b[?1049h", resize: [2]int{10, 2}, after: "\x1b[?1049l\x1b[J",
			grid: "F F", screen: "\nprompt", history: "hist|top|",
		},
		{
			name:  "DECSC, a shrink, DECRC, then EL2",
			bytes: parked + "\x1b[4;1H\x1b7", resize: [2]int{10, 2}, after: "\x1b8\x1b[2K",
			grid: "F F", screen: "\n", history: "hist|top|",
		},
		{
			name:  "DECSC, a shrink, DECRC, then EL0 at col 0",
			bytes: parked + "\x1b[4;1H\x1b7", resize: [2]int{10, 2}, after: "\x1b8\x1b[K",
			grid: "F F", screen: "\n", history: "hist|top|",
		},
		// As in xterm, a bottom margin never reaches past the screen, so a scroll cannot run off it.
		{
			name:  "a bottom margin past the screen is clamped: DL at its top",
			bytes: parked + "\x1b[1;20r\x1b[M",
			grid:  "F F F F", screen: "\n\n\n", history: "hist|top",
		},
		{
			name:  "a bottom margin past the screen is clamped: LF scrolls at the last row",
			bytes: parked + "\x1b[1;20r\x1b[20B\n",
			grid:  "F F F F", screen: "\n\n\n", history: "hist|top",
		},
		{
			name:  "a shrink resets the margins",
			bytes: parked + "\x1b[1;4r", resize: [2]int{10, 2}, after: "\x1b[M",
			grid: "F F", screen: "\n", history: "hist|top",
		},
		{
			name:  "a right margin past the screen is clamped: a wrap scrolls into the scrollback",
			bytes: "\x1b[?69h\x1b[1;20s" + thirty + crlf(4),
			grid:  "F F F F", scrollback: "F T T", history: "0123456789|ABCDEFGHIJ|KLMNOPQRST",
		},
		{
			name:  "ED3 drops the scrollback flags with their lines",
			bytes: thirty + crlf(4) + "\x1b[3J" + crlf(2), grid: "F F F F", scrollback: "F F",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vm := newVTModel(10, 4, cmp.Or(tc.maxLines, 100))
			t.Cleanup(vm.Close)
			vm.Write([]byte(tc.bytes))
			if tc.resize != [2]int{} {
				vm.Resize(tc.resize[0], tc.resize[1])
			}
			vm.Write([]byte(tc.after))
			assert.False(t, vm.Degraded(), "the emulator must not panic")

			grid := make([]string, vm.emu.Height())
			for y := range grid {
				grid[y] = flagLetter(vm.emu.RowWrapped(y))
			}
			assert.Equal(t, tc.grid, strings.Join(grid, " "), "grid flags")
			assert.False(t, vm.emu.RowWrapped(-1) || vm.emu.RowWrapped(vm.emu.Height()), "a row off the grid reads unwrapped")
			if tc.scrollback != "" {
				sb := make([]string, vm.emu.ScrollbackLen())
				for i := range sb {
					sb[i] = flagLetter(vm.emu.ScrollbackRowWrapped(i))
				}
				assert.Equal(t, tc.scrollback, strings.Join(sb, " "), "scrollback flags")
			}
			if tc.screen != "" {
				assert.Equal(t, tc.screen, vm.ScreenText(), "screen")
			}
			if tc.history != "" {
				assert.Equal(t, tc.history, strings.Join(scrollbackStrings(vm), "|"), "scrollback text")
			}
		})
	}
}

// TestResizeReflow pins a resize to xterm.js 6.0.0's Buffer.resize: every expectation is what
// xterm shows after the same bytes and resizes.
func TestResizeReflow(t *testing.T) {
	const (
		thirty = "0123456789ABCDEFGHIJKLMNOPQRST"
		t36    = thirty + "UVWXYZ\r\nhello\r\n" // a 4-row chain, then a short line and the cursor
		tall   = "x"                            // repeated to a chain taller than the viewport
		main   = "0123456789ABCDEFGHIJ\r\n\x1b[?1049h\x1b[H0123456789ABCDEFGHIJ"
	)
	tallChain := strings.Repeat(tall, 60) + "\r\ny"
	tests := []struct {
		name     string
		bytes    string
		size     [2]int // cols, rows; zero means 10x4
		maxLines int    // zero means 100
		resizes  [][2]int
		after    string
		flags    string
		history  string // scrollback text joined by "|"
		screen   string
		cursor   string // "x,y"; empty means not checked
	}{
		{
			name: "a shrink rewraps every chain but the cursor's", bytes: t36, resizes: [][2]int{{6, 4}},
			flags: "grid T T F F | scrollback F T T T", history: "012345|6789AB|CDEFGH|IJKLMN",
			screen: "OPQRST\nUVWXYZ\nhello\n", cursor: "0,3",
		},
		{
			name: "a grow rejoins them", bytes: t36, resizes: [][2]int{{15, 4}},
			flags: "grid T T F F | scrollback F", history: "0123456789ABCDE",
			screen: "FGHIJKLMNOPQRST\nUVWXYZ\nhello\n", cursor: "0,3",
		},
		{
			name: "toggling back restores the model", bytes: t36, resizes: [][2]int{{6, 4}, {15, 4}, {10, 4}},
			flags: "grid T T F F | scrollback F T", history: "0123456789|ABCDEFGHIJ",
			screen: "KLMNOPQRST\nUVWXYZ\nhello\n", cursor: "0,3",
		},
		{
			name: "the cursor's chain is only truncated", bytes: thirty, resizes: [][2]int{{6, 4}},
			flags: "grid F T T F | scrollback", screen: "012345\nABCDEF\nKLMNOP\n", cursor: "5,2",
		},
		{
			name: "wide glyphs crossing the new edge move whole on a shrink", bytes: "01234日本語\r\nx", resizes: [][2]int{{6, 4}},
			flags: "grid F T F F | scrollback", screen: "01234\n日本語\nx\n", cursor: "1,2",
		},
		{
			name: "a wide glyph at the new edge moves whole on a grow", bytes: "abcde日f\r\nx", size: [2]int{4, 4},
			resizes: [][2]int{{6, 4}}, flags: "grid F T F F | scrollback", screen: "abcde\n日f\nx\n", cursor: "1,2",
		},
		{
			name: "a blank continuation vanishes on a grow", bytes: "0123456789A\b\x1b[X\r\nx", resizes: [][2]int{{15, 4}},
			flags: "grid F F F F | scrollback", screen: "0123456789\nx\n\n", cursor: "1,1",
		},
		{
			name: "a chain taller than the viewport rewraps through the scrollback", bytes: tallChain, resizes: [][2]int{{6, 4}},
			flags:   "grid T T T F | scrollback F T T T T T T",
			history: strings.Repeat("xxxxxx|", 6) + "xxxxxx", screen: "xxxxxx\nxxxxxx\nxxxxxx\ny", cursor: "1,3",
		},
		{
			name: "a chain taller than the viewport rejoins on a grow", bytes: tallChain, resizes: [][2]int{{15, 4}},
			flags: "grid T T T F | scrollback F", history: strings.Repeat(tall, 15),
			screen: strings.Repeat(strings.Repeat(tall, 15)+"\n", 3) + "y", cursor: "1,3",
		},
		{
			name: "the cap drops the oldest rows a shrink adds", bytes: thirty + "\r\n\r\n\r\n", maxLines: 3,
			resizes: [][2]int{{6, 4}}, flags: "grid T F F F | scrollback F T T", history: "6789AB|CDEFGH|IJKLMN",
			screen: "OPQRST\n\n\n", cursor: "0,3",
		},
		{
			name: "the alternate screen is truncated, never reflowed", bytes: main, resizes: [][2]int{{6, 4}},
			flags: "grid F T F F | scrollback F", history: "012345", screen: "012345\nABCDEF\n\n", cursor: "5,1",
		},
		{
			name: "the primary screen under it is reflowed", bytes: main, resizes: [][2]int{{6, 4}}, after: "\x1b[?1049l",
			flags: "grid T T T F | scrollback F", history: "012345", screen: "6789AB\nCDEFGH\nIJ\n", cursor: "0,3",
		},
		{
			name: "DECSC below a chain restores to its line", bytes: thirty + "\r\n\x1b7", resizes: [][2]int{{6, 4}},
			after: "\x1b8X", flags: "grid T T T F | scrollback F T", history: "012345|6789AB",
			screen: "CDEFGH\nIJKLMN\nOPQRST\nX", cursor: "1,3",
		},
		{
			name: "DECSC above a chain moves by the rows it adds", bytes: "\x1b7\r\n" + thirty + "\r\n", resizes: [][2]int{{6, 4}},
			after: "\x1b8X", flags: "grid T T T F | scrollback F F T", history: "|012345|6789AB",
			screen: "XDEFGH\nIJKLMN\nOPQRST\n", cursor: "1,0",
		},
		{
			// xterm.js pops a row per row it adds while the cursor is above, losing the chain's head.
			name: "a chain below the cursor pops the rows it adds", bytes: "\x1b[3;1Habcdefghijklmnopqrst\x1b[H",
			resizes: [][2]int{{4, 4}}, flags: "grid F T T T | scrollback", screen: "\nijkl\nmnop\nqrst", cursor: "0,3",
		},
		{
			name: "a pending wrap resumes past the old edge on a grow", bytes: "0123456789", resizes: [][2]int{{15, 4}},
			after: "X", flags: "grid F F F F | scrollback", screen: "0123456789X\n\n\n", cursor: "11,0",
		},
		{
			name: "a pending wrap ends on a shrink", bytes: "0123456789", resizes: [][2]int{{6, 4}},
			after: "X", flags: "grid F F F F | scrollback", screen: "01234X\n\n\n",
		},
		{
			name: "a height-only resize reflows nothing", bytes: t36, resizes: [][2]int{{10, 3}},
			flags: "grid T F F | scrollback F T T", history: "0123456789|ABCDEFGHIJ|KLMNOPQRST",
			screen: "UVWXYZ\nhello\n", cursor: "0,2",
		},
		{
			name: "a same-size resize changes nothing", bytes: t36, resizes: [][2]int{{10, 4}},
			flags: "grid T T F F | scrollback F T", history: "0123456789|ABCDEFGHIJ",
			screen: "KLMNOPQRST\nUVWXYZ\nhello\n", cursor: "0,3",
		},
		{
			name: "a 1-column resize truncates and never reflows", bytes: "0123456789ABCDEFGHIJ\r\nx",
			resizes: [][2]int{{1, 4}, {10, 4}}, flags: "grid F T F F | scrollback", screen: "0\nA\nx\n", cursor: "0,2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			size := cmp.Or(tc.size, [2]int{10, 4})
			vm := newVTModel(size[0], size[1], cmp.Or(tc.maxLines, 100))
			t.Cleanup(vm.Close)
			vm.Write([]byte(tc.bytes))
			for _, r := range tc.resizes {
				vm.Resize(r[0], r[1])
				for i := 0; i < vm.emu.ScrollbackLen(); i++ {
					require.LessOrEqual(t, len(vm.emu.ScrollbackLine(i)), r[0], "scrollback line %d after %v", i, r)
				}
			}
			vm.Write([]byte(tc.after))
			require.False(t, vm.Degraded(), "the emulator must not panic")
			assert.Equal(t, tc.flags, wrapFlags(t, vm), "wrap flags")
			assert.Equal(t, tc.history, strings.Join(scrollbackStrings(vm), "|"), "scrollback text")
			assert.Equal(t, tc.screen, vm.ScreenText(), "screen")
			if tc.cursor != "" {
				pos := vm.emu.CursorPosition()
				assert.Equal(t, tc.cursor, fmt.Sprintf("%d,%d", pos.X, pos.Y), "cursor")
			}
		})
	}
}

func flagLetter(wrapped bool) string {
	if wrapped {
		return "T"
	}
	return "F"
}
