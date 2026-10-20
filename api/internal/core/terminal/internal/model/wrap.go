package model

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// appendChainRow appends one row so the client rebuilds its wrap bit. A head (nextWrapped) runs
// untrimmed to the last column with no brk, so the client autowraps onto the next row; any other
// row is trimmed and followed by brk. A writer that addresses rows itself passes brk "".
func appendChainRow(
	dst []byte,
	cells []uv.Cell,
	cols int,
	wrapped, nextWrapped bool,
	brk string,
) []byte {
	if nextWrapped {
		head := padToCols(cells, cols)
		dst, _ = appendLine(dst, head, len(head), false)
		return dst
	}
	dst, end := appendLine(dst, cells, len(cells), true)
	if wrapped && end == 0 {
		dst, end = append(dst, ' '), 1 // the client wraps only when a cell lands on the row
	}
	// The wrap onto a continuation may scroll the client and fill the row with the pen's
	// background, so a short one erases the rest; EL0 at x>0 keeps the wrap bit in xterm.js and
	// x/vt. At the last column the wrap is pending, and x/vt's EL0 would erase that cell.
	if wrapped && end < cols {
		dst = append(dst, ansi.EraseLineRight...)
	}
	return append(dst, brk...)
}

// padToCols extends cells with blanks to cols: x/vt trims a scrollback line's trailing blanks,
// and a head must reach the last column to wrap there. It copies rather than appends, because a
// scrollback line is the emulator's own slice.
func padToCols(
	cells []uv.Cell,
	cols int,
) []uv.Cell {
	if len(cells) >= cols {
		return cells
	}
	out := make([]uv.Cell, cols)
	for i := copy(out, cells); i < cols; i++ {
		out[i] = uv.EmptyCell
	}
	return out
}
