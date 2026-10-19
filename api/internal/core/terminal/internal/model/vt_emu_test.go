package model

import (
	"cmp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRowWrapped pins the emulator's per-row wrap bit to xterm's isWrapped: autowrap sets
// it, the row carries it into the scrollback, and it drops only where xterm drops it.
func TestRowWrapped(t *testing.T) {
	const (
		wrap   = "0123456789ABCDEFGHIJ" // row 0 full, autowraps into row 1
		thirty = "0123456789ABCDEFGHIJKLMNOPQRST"
	)
	crlf := func(n int) string { return strings.Repeat("\r\n", n) }
	tests := []struct {
		name       string
		bytes      string
		resize     [2]int // cols, rows; zero means no resize
		maxLines   int    // scrollback cap; zero means 100
		grid       string
		scrollback string // empty means not checked
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
			name:  "width change drops all",
			bytes: thirty + crlf(3), resize: [2]int{12, 4}, grid: "F F F F", scrollback: "F F",
		},
		{
			name:  "height-only change keeps",
			bytes: thirty + crlf(3), resize: [2]int{10, 6}, grid: "T F F F F F", scrollback: "F T",
		},
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
		{name: "RIS clears", bytes: wrap + "\x1bc", grid: "F F F F"},
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

			grid := make([]string, vm.emu.Height())
			for y := range grid {
				grid[y] = flagLetter(vm.emu.RowWrapped(y))
			}
			assert.Equal(t, tc.grid, strings.Join(grid, " "), "grid flags")
			if tc.scrollback == "" {
				return
			}
			sb := make([]string, vm.emu.ScrollbackLen())
			for i := range sb {
				sb[i] = flagLetter(vm.emu.ScrollbackRowWrapped(i))
			}
			assert.Equal(t, tc.scrollback, strings.Join(sb, " "), "scrollback flags")
		})
	}
}

func flagLetter(wrapped bool) string {
	if wrapped {
		return "T"
	}
	return "F"
}
