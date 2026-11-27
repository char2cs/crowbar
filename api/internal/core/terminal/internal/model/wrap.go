package model

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

type chainNext uint8

const (
	nextBreaks chainNext = iota
	nextWraps
	nextWrapsWide
)

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
		if next == nextWrapsWide && n == cols && noGlyph(&head[n-1]) {
			n--
		}
		dst, _ = appendLine(dst, head, n, false)
		return dst
	}
	dst, end := appendLine(dst, cells, len(cells), true)
	if wrapped && end == 0 {
		dst, end = append(dst, ' '), 1
	}
	if wrapped && end < cols {
		dst = append(dst, ansi.EraseLineRight...)
	}
	return append(dst, brk...)
}

func noGlyph(
	c *uv.Cell,
) bool {
	return c.Width == 1 && (c.Content == "" || c.Content == " ")
}

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
