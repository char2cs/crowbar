package model

import uv "github.com/charmbracelet/ultraviolet"

// encodeChainRow encodes one row so the client rebuilds its wrap bit. A head (nextWrapped) runs
// untrimmed to the last column with no brk, so the client autowraps onto the next row; any other
// row is trimmed and followed by brk, and a continuation (wrapped) keeps at least one cell.
func encodeChainRow(
	cells []uv.Cell,
	cols int,
	wrapped, nextWrapped bool,
	brk string,
) string {
	if nextWrapped {
		head := padToCols(cells, cols)
		return encodeLine(head, len(head), false)
	}
	row := encodeLine(cells, len(cells), true)
	if row == "" && wrapped {
		row = " " // the client wraps only when a cell lands on the row
	}
	return row + brk
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
