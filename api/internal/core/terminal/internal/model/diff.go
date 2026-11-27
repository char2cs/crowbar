package model

import (
	"bytes"
	"hash/fnv"
	"strconv"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func cup(row, col int) string {
	return "\x1b[" + strconv.Itoa(row) + ";" + strconv.Itoa(col) + "H"
}

type DiffEmitter struct {
	valid bool

	cols, rows     int
	alt            bool
	scrollbackLen  int
	scrollbackTail uint64
	lastGrid       [][]uv.Cell
	lastWrapped    []bool
	lastCursor     uv.Position
	chrome         chromeBase
}

const rotationScanLimit = 256

func NewDiffEmitter() *DiffEmitter {
	return &DiffEmitter{}
}

func (e *DiffEmitter) Invalidate() {
	e.valid = false
}

func (e *DiffEmitter) Prime(m TerminalModel) {
	vm := m.(*vtModel)
	e.cols, e.rows = vm.emu.Width(), vm.emu.Height()
	e.alt = vm.emu.IsAltScreen()
	e.scrollbackLen = vm.emu.ScrollbackLen()
	if e.scrollbackLen > 0 {
		e.scrollbackTail = scrollbackLineHash(vm, e.scrollbackLen-1)
	} else {
		e.scrollbackTail = 0
	}
	e.lastCursor = vm.emu.CursorPosition()
	e.lastGrid = snapshotGrid(vm.emu, e.cols, e.rows)
	e.lastWrapped = make([]bool, e.rows)
	for y := range e.lastWrapped {
		e.lastWrapped[y] = vm.emu.RowWrapped(y)
	}
	e.chrome = captureChrome(&vm.shadow)
	e.valid = true
}

//nolint:gocyclo // Emit is a linear sequence of independent bail-to-keyframe guards; decomposing them would obscure the documented step order without lowering real complexity.
func (e *DiffEmitter) Emit(m TerminalModel) (data []byte, needKeyframe bool) {
	vm := m.(*vtModel)
	cols, rows := vm.emu.Width(), vm.emu.Height()
	alt := vm.emu.IsAltScreen()
	sbLen := vm.emu.ScrollbackLen()

	if !e.valid || cols != e.cols || rows != e.rows || alt != e.alt || sbLen < e.scrollbackLen {
		return nil, true
	}

	sh := &vm.shadow
	if sh.scrollRegionSet != e.chrome.scrollRegionSet ||
		sh.scrollTop != e.chrome.scrollTop ||
		sh.scrollBottom != e.chrome.scrollBottom ||
		sh.modes[6] != e.chrome.origin {
		return nil, true
	}

	sbStart, ok := e.scrollbackNewStart(vm, sbLen)
	if !ok {
		return nil, true
	}
	newLines := sbLen - sbStart

	if newLines > 0 && !e.alt {
		if sh.scrollRegionSet || sh.modes[6] {
			return nil, true
		}
		for y := range e.lastGrid {
			e.lastGrid[y] = e.lastGrid[y][:0]
		}
	}

	var b bytes.Buffer
	e.writeScrollbackDelta(&b, vm.emu, sbStart, sbLen, cols, rows)
	dirty := e.writeScreenDiff(&b, vm, cols, rows)
	e.writeCursorDelta(&b, vm, dirty)
	e.writeChromeDelta(&b, sh)

	e.scrollbackLen = sbLen
	if newLines > 0 && sbLen > 0 {
		e.scrollbackTail = scrollbackLineHash(vm, sbLen-1)
	}
	e.lastCursor = vm.emu.CursorPosition()
	e.chrome = e.recaptureChrome(sh)
	if b.Len() == 0 {
		return nil, false
	}
	return b.Bytes(), false
}

func (e *DiffEmitter) scrollbackNewStart(vm *vtModel, sbLen int) (start int, ok bool) {
	if e.alt {
		return sbLen, true
	}
	if e.scrollbackLen == 0 {
		return 0, true
	}
	if sbLen < vm.scrollbackLines {
		return e.scrollbackLen, true
	}
	lo := sbLen - 1 - rotationScanLimit
	if lo < 0 {
		lo = 0
	}
	for i := sbLen - 1; i >= lo; i-- {
		if scrollbackLineHash(vm, i) == e.scrollbackTail {
			return i + 1, true
		}
	}
	return 0, false
}

func scrollbackLineHash(vm *vtModel, idx int) uint64 {
	line := vm.emu.ScrollbackLine(idx)
	h := fnv.New64a()
	h.Write([]byte(encodeLine(line, len(line), true))) //nolint:gosec // G104: a hash.Hash never fails a Write
	return h.Sum64()
}

func EmitterGridBytes(cols, rows int) int64 {
	return int64(cols) * int64(rows) * 32
}

func (e *DiffEmitter) writeScrollbackDelta(
	b *bytes.Buffer,
	emu emulator,
	start int,
	sbLen int,
	cols int,
	rows int,
) {
	if e.alt || start >= sbLen {
		return
	}
	wrapOff := e.clientAutowrapOff()
	if wrapOff {
		b.WriteString(ansi.SetModeAutoWrap)
	}
	top := emu.RowWrapped(0)
	b.WriteString(cup(1, 1))
	for i := start; i < sbLen; i++ {
		wrapped := emu.ScrollbackRowWrapped(i) && (i > start || e.lastWrapped[0])
		if !wrapped && i > start {
			b.WriteString("\r\n")
		}
		if !wrapped {
			b.WriteString(ansi.EraseLineRight)
		}
		next := chainNextOf(top, emu.CellAt(0, 0))
		if i+1 < sbLen {
			next = chainNextOf(emu.ScrollbackRowWrapped(i+1), emu.ScrollbackLine(i+1).At(0))
		}
		b.Write(appendChainRow(b.AvailableBuffer(), emu.ScrollbackLine(i), cols, wrapped, next, ""))
	}
	k := min(sbLen-start, rows)
	if top {
		row := snapshotRowInto(emu, e.lastGrid[0], cols, 0)
		b.Write(appendChainRow(b.AvailableBuffer(), row, cols, true, nextBreaks, ""))
		e.lastGrid[0] = row[:0]
		k = min(sbLen-start, rows-1)
	}
	b.WriteString(cup(rows, 1))
	for range k {
		b.WriteString("\r\n")
	}
	if wrapOff {
		b.WriteString(ansi.ResetModeAutoWrap)
	}
	e.lastWrapped[0] = top
}

func (e *DiffEmitter) writeScreenDiff(
	b *bytes.Buffer,
	vm *vtModel,
	cols int,
	rows int,
) bool {
	dirty := false
	for y, end := 0, 0; y < rows; y = end + 1 {
		end = chainEnd(vm.emu, y, rows)
		from := e.flowStart(vm.emu, y, end, cols)
		if from > end {
			continue
		}
		dirty = true
		e.writeChain(b, vm.emu, from, end, cols)
	}
	return dirty
}

func (e *DiffEmitter) flowStart(
	emu emulator,
	y0, end, cols int,
) int {
	for y := y0; y <= end; y++ {
		if emu.RowWrapped(y) != e.lastWrapped[y] {
			return y0
		}
		if !rowEqualsGrid(emu, e.lastGrid[y], cols, y) {
			return y
		}
	}
	return end + 1
}

func (e *DiffEmitter) writeChain(
	b *bytes.Buffer,
	emu emulator,
	y0, end, cols int,
) {
	continuation := emu.RowWrapped(y0)
	wrapOff := end > y0 && e.clientAutowrapOff()
	if wrapOff {
		b.WriteString(ansi.SetModeAutoWrap)
	}
	lift := e.regionEndsInside(y0, end)
	if lift {
		b.WriteString(ansi.SetTopBottomMargins(0, 0))
	}
	b.Grow((end - y0 + 1) * cols)
	b.WriteString(cup(y0+1, 1))
	if !continuation {
		b.WriteString(ansi.EraseLineRight)
	}
	for y := y0; y <= end; y++ {
		e.lastGrid[y] = snapshotRowInto(emu, e.lastGrid[y], cols, y)
		next := chainNextOf(emu.RowWrapped(y+1), emu.CellAt(0, y+1))
		b.Write(appendChainRow(b.AvailableBuffer(), e.lastGrid[y], cols, emu.RowWrapped(y), next, ""))
		e.lastWrapped[y] = emu.RowWrapped(y) && (y > y0 || e.lastWrapped[y])
	}
	if lift {
		b.WriteString(ansi.SetTopBottomMargins(e.chrome.scrollTop, e.chrome.scrollBottom))
	}
	if wrapOff {
		b.WriteString(ansi.ResetModeAutoWrap)
	}
}

func (e *DiffEmitter) writeCursorDelta(
	b *bytes.Buffer,
	vm *vtModel,
	dirty bool,
) {
	cur := vm.emu.CursorPosition()
	if !dirty && cur == e.lastCursor {
		return
	}
	b.WriteString(cup(cur.Y+1, cur.X+1))
}

func snapshotGrid(emu emulator, cols, rows int) [][]uv.Cell {
	grid := make([][]uv.Cell, rows)
	for y := 0; y < rows; y++ {
		grid[y] = snapshotRow(emu, cols, y)
	}
	return grid
}

func snapshotRow(emu emulator, cols, y int) []uv.Cell {
	return snapshotRowInto(emu, nil, cols, y)
}

func snapshotRowInto(emu emulator, dst []uv.Cell, cols, y int) []uv.Cell {
	if cap(dst) < cols {
		dst = make([]uv.Cell, cols)
	} else {
		dst = dst[:cols]
	}
	for x := 0; x < cols; x++ {
		if c := emu.CellAt(x, y); c != nil {
			dst[x] = *c
		} else {
			dst[x] = uv.EmptyCell
		}
	}
	return dst
}

func rowEqualsGrid(emu emulator, lastRow []uv.Cell, cols, y int) bool {
	if lastRow == nil || len(lastRow) != cols {
		return false
	}
	for x := 0; x < cols; x++ {
		c := emu.CellAt(x, y)
		if c == nil {
			if !uv.EmptyCell.Equal(&lastRow[x]) {
				return false
			}
			continue
		}
		if !c.Equal(&lastRow[x]) {
			return false
		}
	}
	return true
}
