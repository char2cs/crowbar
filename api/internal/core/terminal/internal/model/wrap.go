package model

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// chainNext is what follows a row: a line of its own, or a continuation of this one.
type chainNext uint8

const (
	nextBreaks    chainNext = iota // the next row starts a line of its own
	nextWraps                      // the next row continues this one
	nextWrapsWide                  // the next row continues this one and starts with a wide glyph
)

// chainNextOf classifies the row after this one from its wrap bit and its first cell.
func chainNextOf(
	wraps bool,
	first *uv.Cell,
) chainNext {
	switch {
	case !wraps:
		return nextBreaks
	case first != nil && first.Width > 1:
		return nextWrapsWide
	default:
		return nextWraps
	}
}

// appendChainRow appends one row so the client rebuilds its wrap bit. A head (next wraps) runs
// untrimmed to the last column with no brk, so the client autowraps onto the next row; any other
// row is trimmed and followed by brk. A writer that addresses rows itself passes brk "".
func appendChainRow(
	dst []byte,
	cells []uv.Cell,
	cols int,
	wrapped bool,
	next chainNext,
	brk string,
) []byte {
	if next != nextBreaks {
		head := padToCols(cells, cols)
		n := len(head)
		// The client wraps a wide glyph early and blanks the last cell itself; a printed space
		// there would count as content to xterm's next reflow.
		if next == nextWrapsWide && n == cols && noGlyph(&head[n-1]) {
			n--
		}
		dst, _ = appendLine(dst, head, n, false)
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

// noGlyph reports a one-column cell with no glyph, whatever its pen.
func noGlyph(
	c *uv.Cell,
) bool {
	return c.Width == 1 && (c.Content == "" || c.Content == " ")
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

// chainEnd returns the last grid row of the chain that starts at row y: every row below it that
// continues it by autowrap, up to the last row.
func chainEnd(
	emu emulator,
	y int,
	rows int,
) int {
	for y+1 < rows && emu.RowWrapped(y+1) {
		y++
	}
	return y
}
