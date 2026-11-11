package model

import (
	"bytes"
	"hash/fnv"
	"strconv"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// cup returns an always-explicit CUP sequence (1-based): ansi.CursorPosition compacts
// default parameters, and the emit contract wants deterministic bytes clients can match.
func cup(row, col int) string {
	return "\x1b[" + strconv.Itoa(row) + ";" + strconv.Itoa(col) + "H"
}

// DiffEmitter turns model changes into minimal incremental ANSI for live clients; an
// unprimed emitter, a resize, an alt-screen flip or a scrollback shrink demands a keyframe,
// since reset+redraw beats a worst-case diff. Not goroutine-safe: the session's lock serialises it.
type DiffEmitter struct {
	valid bool

	cols, rows    int
	alt           bool
	scrollbackLen int
	// scrollbackTail hashes the last scrollback line at the previous Emit/Prime: once x/vt's
	// ring saturates its length plateaus, so this anchor recovers the new-line boundary.
	scrollbackTail uint64
	lastGrid       [][]uv.Cell
	// lastWrapped is each grid row's wrap bit as the client holds it.
	lastWrapped []bool
	lastCursor  uv.Position
	chrome      chromeBase
}

// rotationScanLimit bounds the anchor scan; a bigger gap between emits is rare and a
// keyframe is the correct, cheap recovery.
const rotationScanLimit = 256

// NewDiffEmitter returns an unprimed emitter (first Emit demands a keyframe).
func NewDiffEmitter() *DiffEmitter {
	return &DiffEmitter{}
}

// Invalidate discards the diff base; the next Emit demands a keyframe.
func (e *DiffEmitter) Invalidate() {
	e.valid = false
}

// Prime captures the model's current grid/cursor/counters as the diff base.
// Call immediately after emitting a keyframe serialized from the same locked
// model state.
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

// Emit returns the ANSI since the last Prime/Emit and re-bases, or needKeyframe on: no base, resize,
// alt-screen flip, scrollback shrink, DECSTBM/DECOM change, anchor loss, or lines committed under an
// already-active region/origin mode (the client's CUP+LF trick fails there).
//
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
		// The diff's absolute CUPs assume no scroll region and no origin mode; only a keyframe
		// reproduces a change to either.
		return nil, true
	}

	// Locate the first scrollback line committed since the last emit, tolerant
	// of x/vt's ring rotation at cap (Finding A: ScrollbackLen() plateaus once
	// saturated, so a length compare alone would starve the delta path).
	sbStart, ok := e.scrollbackNewStart(vm, sbLen)
	if !ok {
		// The rotation anchor scrolled out of the scan window; resync.
		return nil, true
	}
	newLines := sbLen - sbStart

	if newLines > 0 && !e.alt {
		// An active region or origin mode set before Prime confines the client's CUP+LF scroll,
		// so committed lines would never reach client history; a keyframe re-asserts both.
		if sh.scrollRegionSet || sh.modes[6] {
			return nil, true
		}
		// The client screen scrolls while absorbing the delta; every row's
		// on-screen identity moves, so rebuild the whole viewport after.
		for y := range e.lastGrid {
			// Truncate instead of nilling: the zero-length row still forces a rewrite, and the
			// backing array survives for snapshotRowInto to refill.
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
		// The tail moved; re-anchor. Skipped when nothing scrolled so the echo
		// path stays hash-free (the anchor is still valid).
		e.scrollbackTail = scrollbackLineHash(vm, sbLen-1)
	}
	e.lastCursor = vm.emu.CursorPosition()
	e.chrome = e.recaptureChrome(sh)
	if b.Len() == 0 {
		return nil, false
	}
	// b.Bytes() aliases the local buffer, which is discarded when Emit returns —
	// so the frame is handed off with no extra copy (unlike []byte(b.String())).
	return b.Bytes(), false
}

// scrollbackNewStart returns the first scrollback index committed since the last emit, scanning
// back for the anchor only once the ring saturated: below that the old length is exact, and
// scanning would misanchor on duplicate lines. ok=false means the anchor rotated out: keyframe.
func (e *DiffEmitter) scrollbackNewStart(vm *vtModel, sbLen int) (start int, ok bool) {
	if e.alt {
		return sbLen, true // alt screen has no scrollback
	}
	if e.scrollbackLen == 0 {
		return 0, true
	}
	if sbLen < vm.scrollbackLines {
		// Never saturated means nothing was evicted, so the old length is the exact boundary;
		// scanning could only misanchor on duplicate content.
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

// scrollbackLineHash is the FNV-1a anchor hash. In a saturated ring a collision or a duplicate
// line can misplace the boundary; that self-heals on the next keyframe and is accepted.
func scrollbackLineHash(vm *vtModel, idx int) uint64 {
	line := vm.emu.ScrollbackLine(idx)
	h := fnv.New64a()
	h.Write([]byte(encodeLine(line, len(line), true))) //nolint:gosec // G104: a hash.Hash never fails a Write
	return h.Sum64()
}

// EmitterGridBytes estimates the emitter's retained diff base (~32B/cell) from the model's
// dimensions, so the memory-ceiling arithmetic sees a stable size from spawn.
func EmitterGridBytes(cols, rows int) int64 {
	return int64(cols) * int64(rows) * 32
}

// writeScrollbackDelta writes the committed lines as one flow from CUP(1,1), continuations unaddressed
// so the client's own autowrap sets each wrap bit, then scrolls them out with LFs from the bottom row;
// the screen diff that follows repaints the viewport.
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
			b.WriteString(ansi.EraseLineRight) // at column 0, EL0 also ends the client row's wrap
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
		e.lastGrid[0] = row[:0] // still the forced-rewrite marker Emit set
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

// writeScreenDiff rewrites every chain of grid rows holding a changed row or
// wrap bit, from the row flowStart picks to the chain's end (writeChain), and
// updates the diff base. Returns whether anything was written.
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

// flowStart returns end+1 for a clean chain, else the row its rewrite starts on: the first changed
// row when the client holds that row's wrap bit, or the chain's first row, whose flow sets the bit.
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

// writeChain writes rows y0..end as one flow from column 1 of y0 (pen reset per row via
// appendChainRow), so the client's autowrap sets the flag of every row below y0. EL0 at column 0
// clears a head's flag; a continuation is written without it, keeping the flag the client holds.
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
	b.Grow((end - y0 + 1) * cols) // a head writes every column: one growth, not one per row
	b.WriteString(cup(y0+1, 1))
	if !continuation {
		b.WriteString(ansi.EraseLineRight)
	}
	for y := y0; y <= end; y++ {
		// Snapshot first, then encode the snapshot: one materialisation serves the wire and the base.
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

// writeCursorDelta repositions the client cursor to the model's position when
// it moved or when screen rewrites displaced it.
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

// snapshotRowInto fills dst with row y, reusing its array when large enough; a nil dst
// (the invalidation marker set in Emit) allocates, keeping the force-rewrite meaning.
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

// rowEqualsGrid compares row y against lastRow without allocating; a nil lastRow (the
// invalidation marker) is never equal, forcing a rewrite.
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
