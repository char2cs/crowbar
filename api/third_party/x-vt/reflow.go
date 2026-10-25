// Ported from xterm.js 6.0.0 (src/common/buffer/Buffer.ts resize, _reflowLarger, _reflowSmaller;
// BufferReflow.ts), Copyright (c) 2017-2019 The xterm.js authors, MIT License: see
// licenses/xtermjs-MIT.txt at the repository root. Names follow xterm's.

package vt

import (
	"slices"

	uv "github.com/charmbracelet/ultraviolet"
)

// reflowBuf is xterm.js's Buffer mid-resize: the scrollback and the grid as one list, ybase the
// grid's first row, savedY absolute. A line may end early; past its end lie blanks (cellAt).
type reflowBuf struct {
	lines                       []uv.Line
	wrapped                     []bool // wrapped[i] reports that line i continues line i-1
	ybase, x, y, savedX, savedY int
	maxLength                   int // rows + scrollback, the list's cap
}

// resizeLines resizes the screen as xterm.js resizes a buffer, reflowing its soft-wrapped lines
// when reflow is set. pending reports a wrap pending at the cursor. It returns the cursor.
func (s *Screen) resizeLines(width, height int, pending, reflow bool) (x, y int) {
	oldCols, oldRows := s.buf.Width(), s.buf.Height()
	b := s.combine(pending, height)
	addToY := b.stepRows(oldRows, height)
	if b.maxLength < oldRows+s.scrollback.MaxLines() {
		b.capLength()
	}
	b.clampCursor(width, height, addToY)
	// xterm.js never runs below 2 columns, and reflowSmaller would not end at 1.
	switch {
	case !reflow || min(width, oldCols) < 2:
	case width > oldCols:
		b.reflowLarger(oldCols, width, height)
	case width < oldCols:
		b.reflowSmaller(oldCols, width, height)
	}
	if width < oldCols {
		b.truncateCols(width)
	}
	s.writeBack(&b, width, height)
	return s.cur.X, s.cur.Y
}

// stepRows adds or removes rows: a grow pulls history down while the cursor is on the last line,
// a shrink drops rows below the cursor, else scrolls the top into history. It returns addToY.
func (b *reflowBuf) stepRows(oldRows, newRows int) int {
	addToY := 0
	for y := oldRows; y < newRows; y++ {
		switch {
		case len(b.lines) >= newRows+b.ybase:
		case b.ybase > 0 && len(b.lines) <= b.ybase+b.y+addToY+1:
			b.ybase--
			addToY++
		default:
			b.lines, b.wrapped = append(b.lines, uv.Line{}), append(b.wrapped, false)
		}
	}
	for y := oldRows; y > newRows; y-- {
		switch {
		case len(b.lines) <= newRows+b.ybase:
		case len(b.lines) > b.ybase+b.y+1:
			b.lines, b.wrapped = b.lines[:len(b.lines)-1], b.wrapped[:len(b.wrapped)-1]
		default:
			b.ybase++
		}
	}
	return addToY
}

// capLength trims the oldest lines past maxLength.
func (b *reflowBuf) capLength() {
	if n := len(b.lines) - b.maxLength; n > 0 {
		b.lines, b.wrapped = b.lines[n:], b.wrapped[n:]
		b.ybase, b.savedY = max(b.ybase-n, 0), max(b.savedY-n, 0)
	}
}

// clampCursor keeps the cursor on the screen.
func (b *reflowBuf) clampCursor(cols, rows, addToY int) {
	b.x, b.y, b.savedX = min(b.x, cols-1), min(b.y, rows-1)+addToY, min(b.savedX, cols-1)
}

// reflowLarger rejoins the chains a wider screen fits (Buffer._reflowLarger).
func (b *reflowBuf) reflowLarger(oldCols, newCols, newRows int) {
	toRemove := largerLinesToRemove(b.lines, b.wrapped, oldCols, newCols, b.ybase+b.y)
	if len(toRemove) == 0 {
		return
	}
	removed := b.removeLines(toRemove)
	for range removed {
		if b.ybase > 0 {
			b.ybase--
			continue
		}
		b.y = max(b.y-1, 0)
		if len(b.lines) < newRows {
			b.lines, b.wrapped = append(b.lines, uv.Line{}), append(b.wrapped, false)
		}
	}
	b.savedY = max(b.savedY-removed, 0)
}

// largerLinesToRemove rejoins each chain but the cursor's; it returns (index, count) pairs to drop.
func largerLinesToRemove(lines []uv.Line, wrapped []bool, oldCols, newCols, cursorY int) []int {
	var toRemove []int
	for y := 0; y < len(lines)-1; y++ {
		i := y + 1
		if !wrapped[i] {
			continue
		}
		for i < len(lines) && wrapped[i] {
			i++
		}
		if cursorY < y || cursorY >= i {
			chain := lines[y:i]
			for k := range chain {
				chain[k] = fitLine(chain[k], oldCols)
			}
			if n := rejoinChain(chain, oldCols, newCols); n > 0 {
				toRemove = append(toRemove, i-n, n)
			}
		}
		y = i - 1
	}
	return toRemove
}

// rejoinChain moves a chain's cells up to fill rows of newCols, a wide glyph that would end
// past the last column moving whole to the next row. It returns the rows left empty at its end.
func rejoinChain(chain []uv.Line, oldCols, newCols int) int {
	chain[0] = fitLine(chain[0], newCols) // a row widens only once it is written to
	destLine, destCol := 0, wrappedTrimmedLength(chain, 0, oldCols)
	srcLine, srcCol := 1, 0
	for srcLine < len(chain) {
		srcLen := wrappedTrimmedLength(chain, srcLine, oldCols)
		n := min(srcLen-srcCol, newCols-destCol)
		copy(chain[destLine][destCol:destCol+n], chain[srcLine][srcCol:srcCol+n])
		if destCol += n; destCol == newCols {
			destLine, destCol = destLine+1, 0
			chain[destLine] = fitLine(chain[destLine], newCols)
		}
		if srcCol += n; srcCol == srcLen {
			srcLine, srcCol = srcLine+1, 0
		}
		if destCol == 0 && destLine != 0 && chain[destLine-1][newCols-1].Width == 2 {
			chain[destLine][0] = chain[destLine-1][newCols-1]
			chain[destLine-1][newCols-1] = uv.EmptyCell
			destCol = 1
		}
	}
	replaceCells(chain[destLine], destCol)
	count := 0
	for i := len(chain) - 1; i > 0 && (i > destLine || trimmedLength(chain[i]) == 0); i-- {
		count++
	}
	return count
}

// removeLines drops toRemove's (index, count) pairs and returns how many lines went.
func (b *reflowBuf) removeLines(toRemove []int) int {
	kept, removed, next := 0, 0, 0
	for i := 0; i < len(b.lines); i++ {
		if next < len(toRemove) && toRemove[next] == i {
			i += toRemove[next+1] - 1
			removed += toRemove[next+1]
			next += 2
			continue
		}
		b.lines[kept], b.wrapped[kept] = b.lines[i], b.wrapped[i]
		kept++
	}
	b.lines, b.wrapped = b.lines[:kept], b.wrapped[:kept]
	return removed
}

// insertion is a run of new rows that goes before line start.
type insertion struct {
	start int
	lines []uv.Line
}

// reflowSmaller rewraps, bottom up, each line too long but the cursor's, then splices rows in.
func (b *reflowBuf) reflowSmaller(oldCols, newCols, newRows int) {
	var toInsert []insertion
	count := 0
	for y := len(b.lines) - 1; y >= 0; y-- {
		if y >= len(b.lines) || !b.wrapped[y] && trimmedLength(b.lines[y]) <= newCols {
			continue // pops can leave y past the end, where xterm.js reads an undefined line
		}
		end := y + 1
		for b.wrapped[y] && y > 0 {
			y--
		}
		if cursor := b.ybase + b.y; cursor >= y && cursor < end {
			continue
		}
		added := b.rewrapChain(y, end, oldCols, newCols)
		if len(added) > 0 {
			toInsert = append(toInsert, insertion{start: end + count, lines: added})
			count += len(added)
		}
		trimmed := max(0, len(b.lines)-b.maxLength+len(added)) // the oldest lines the cap drops
		if b.ybase == 0 && b.y != len(b.lines)-1 {
			trimmed = max(0, b.y-b.maxLength+len(added))
		}
		b.adjustSmaller(len(added)-trimmed, newRows, count)
		b.savedY = min(b.savedY+len(added), b.ybase+newRows-1)
	}
	if len(toInsert) > 0 {
		b.insertLines(toInsert, count)
	}
}

// rewrapChain rewraps lines[start:end] to newCols, copying backwards, and returns the rows it adds.
func (b *reflowBuf) rewrapChain(start, end, oldCols, newCols int) []uv.Line {
	for k := start; k < end; k++ {
		b.lines[k] = fitLine(b.lines[k], oldCols)
	}
	lastLen := trimmedLength(b.lines[end-1])
	lengths := smallerLineLengths(b.lines[start:end], oldCols, newCols)
	chain := append(make([]uv.Line, 0, max(len(lengths), end-start)), b.lines[start:end]...)
	slab := slices.Repeat(uv.Line{uv.EmptyCell}, max(len(lengths)-len(chain), 0)*newCols)
	for i := 0; len(chain) < len(lengths); i += newCols {
		chain = append(chain, slab[i:i+newCols:i+newCols])
	}
	destLine := len(lengths) - 1
	destCol := lengths[destLine]
	if destCol == 0 {
		destLine--
		destCol = lengths[max(destLine, 0)]
	}
	srcLine, srcCol := end-start-1, lastLen
	for srcLine >= 0 && destLine >= 0 {
		n := min(srcCol, destCol)
		copy(chain[destLine][destCol-n:destCol], chain[srcLine][srcCol-n:srcCol])
		if destCol -= n; destCol == 0 {
			destLine--
			destCol = lengths[max(destLine, 0)]
		}
		if srcCol -= n; srcCol == 0 {
			srcLine--
			srcCol = wrappedTrimmedLength(chain, max(srcLine, 0), oldCols)
		}
	}
	for i, n := range lengths { // a row short of newCols lost a wide glyph to the next one
		if n < newCols {
			chain[i][n] = uv.EmptyCell
		}
	}
	return chain[end-start:]
}

// adjustSmaller moves the viewport down by n rows; count rows are waiting to be inserted.
func (b *reflowBuf) adjustSmaller(n, newRows, count int) {
	for ; n > 0; n-- {
		switch {
		case b.ybase == 0 && b.y < newRows-1:
			b.y++
			b.lines, b.wrapped = b.lines[:len(b.lines)-1], b.wrapped[:len(b.wrapped)-1]
		case b.ybase == 0, b.ybase < min(b.maxLength, len(b.lines)+count)-newRows:
			b.ybase++
		}
	}
}

// insertLines splices each insertion in, keeping the newest maxLength lines.
func (b *reflowBuf) insertLines(toInsert []insertion, count int) {
	n := min(b.maxLength, len(b.lines)+count)
	lines, wrapped := make([]uv.Line, n), make([]bool, n)
	orig, next, inserted := len(b.lines)-1, 0, 0
	for i := n - 1; i >= 0; i-- {
		if next < len(toInsert) && toInsert[next].start > orig+inserted {
			add := toInsert[next].lines
			for k := len(add) - 1; k >= 0 && i >= 0; k-- {
				lines[i], wrapped[i] = add[k], true
				i--
			}
			i++
			inserted += len(add)
			next++
			continue
		}
		if orig < 0 {
			break
		}
		lines[i], wrapped[i] = b.lines[orig], b.wrapped[orig]
		orig--
	}
	b.lines, b.wrapped = lines, wrapped
}

// truncateCols cuts every line to cols, blanking a wide glyph the edge would split.
func (b *reflowBuf) truncateCols(cols int) {
	for i, line := range b.lines {
		if len(line) > cols {
			b.lines[i] = line[:cols]
			if cols > 0 && line[cols-1].Width > 1 {
				line[cols-1].Empty()
			}
		}
	}
}

// smallerLineLengths returns the length of each row a chain rewraps to; a row that would end on
// a wide glyph's first half ends one column short.
func smallerLineLengths(lines []uv.Line, oldCols, newCols int) []int {
	needed := 0
	for i := range lines {
		needed += wrappedTrimmedLength(lines, i, oldCols)
	}
	var lengths []int
	srcCol, srcLine, available := 0, 0, 0
	for available < needed {
		if needed-available < newCols {
			return append(lengths, needed-available)
		}
		srcCol += newCols
		if old := wrappedTrimmedLength(lines, srcLine, oldCols); srcCol > old {
			srcCol, srcLine = srcCol-old, srcLine+1
		}
		n := newCols
		if srcLine < len(lines) && cellAt(lines[srcLine], srcCol-1).Width == 2 {
			srcCol, n = srcCol-1, n-1
		}
		lengths = append(lengths, n)
		available += n
	}
	return lengths
}

// wrappedTrimmedLength is the cells row i of a chain holds: a row's trimmed length if it is the
// last, one short of cols if a wide glyph wrapped early from it, else cols.
func wrappedTrimmedLength(lines []uv.Line, i, cols int) int {
	switch {
	case i >= len(lines):
		return 0
	case i == len(lines)-1:
		return trimmedLength(lines[i])
	}
	last := cellAt(lines[i], cols-1)
	if !hasContent(&last) && last.Width == 1 && cellAt(lines[i+1], 0).Width == 2 {
		return cols - 1
	}
	return cols
}

// trimmedLength is the cells a line holds up to its last content (BufferLine.getTrimmedLength).
func trimmedLength(line uv.Line) int {
	for i := len(line) - 1; i >= 0; i-- {
		if hasContent(&line[i]) {
			return i + max(line[i].Width, 1)
		}
	}
	return 0
}

// hasContent reports a glyph. x/vt cannot tell a printed space from an erased cell, so neither
// counts, where xterm.js counts a printed space (UPSTREAM.md D2).
func hasContent(c *uv.Cell) bool {
	return c.Content != "" && c.Content != " "
}

// replaceCells blanks a line from start to its end, and the head of a wide glyph start cuts.
func replaceCells(line uv.Line, start int) {
	if start > 0 && start <= len(line) && line[start-1].Width == 2 {
		line[start-1] = uv.EmptyCell
	}
	for ; start < len(line); start++ {
		line[start] = uv.EmptyCell
	}
}

// cellAt reads a line the scrollback trimmed: past its end, a trailing wide glyph's rest, blanks.
func cellAt(line uv.Line, x int) uv.Cell {
	n := len(line)
	switch {
	case x >= 0 && x < n:
		return line[x]
	case x >= n && n > 0 && x < n-1+line[n-1].Width:
		return uv.Cell{}
	}
	return uv.EmptyCell
}

// fitLine returns line at exactly cols cells, padded as cellAt reads it or cut. It pads in place
// when the line has room, so it takes a line no one else holds.
func fitLine(line uv.Line, cols int) uv.Line {
	n := len(line)
	if n >= cols {
		return line[:cols]
	}
	padded := slices.Grow(line, cols-n)[:cols]
	for x := n; x < cols; x++ {
		padded[x] = cellAt(line, x)
	}
	return padded
}
