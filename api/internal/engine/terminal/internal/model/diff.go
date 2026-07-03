package model

import (
	"strconv"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// cup returns an always-explicit CUP sequence ("\x1b[<row>;<col>H", 1-based).
// ansi.CursorPosition compacts default parameters ("\x1b[H" for 1;1); the emit
// contract wants a deterministic full form so clients and tests can match the
// exact bytes.
func cup(row, col int) string {
	return "\x1b[" + strconv.Itoa(row) + ";" + strconv.Itoa(col) + "H"
}

// DiffEmitter converts model state changes into minimal incremental ANSI for
// live clients (spec §3.2). It is the streaming counterpart of the Serializer:
// where Serialize emits one full ground-state redraw, Emit produces only what
// changed since the previous Emit/Prime. Like the Serializer it reads the
// concrete model through the package wall (type assertion), never through the
// session-facing interface. Not goroutine-safe; the owning session serialises
// calls under its lock.
//
// Lifecycle: an unprimed (or invalidated) emitter answers Emit with
// needKeyframe=true; the caller then sends a full snapshot (Serializer) and
// calls Prime to re-base. Dimension changes, alt-screen flips and scrollback
// shrinkage (ED3-style clears) are detected in Emit and demand a keyframe the
// same way — a client reset+redraw is both simpler and cheaper than a
// worst-case whole-screen diff with epoch bookkeeping.
type DiffEmitter struct {
	valid bool

	cols, rows    int
	alt           bool
	scrollbackLen int
	lastGrid      [][]uv.Cell
	lastCursor    uv.Position
}

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
	e.lastCursor = vm.emu.CursorPosition()
	e.lastGrid = snapshotGrid(vm.emu, e.cols, e.rows)
	e.valid = true
}

// Emit returns the incremental ANSI since the last Prime/Emit, or
// needKeyframe=true when diffing is impossible (unprimed, invalidated, resize,
// alt-screen flip, scrollback shrink). On success the emitter re-bases itself.
func (e *DiffEmitter) Emit(m TerminalModel) (data []byte, needKeyframe bool) {
	vm := m.(*vtModel)
	cols, rows := vm.emu.Width(), vm.emu.Height()
	alt := vm.emu.IsAltScreen()
	sbLen := vm.emu.ScrollbackLen()

	if !e.valid || cols != e.cols || rows != e.rows || alt != e.alt || sbLen < e.scrollbackLen {
		return nil, true
	}

	if sbLen > e.scrollbackLen {
		// The client screen scrolls while absorbing the delta; every row's
		// on-screen identity moves, so rebuild the whole viewport after.
		for y := range e.lastGrid {
			e.lastGrid[y] = nil // nil never equals a real row → forced rewrite
		}
	}

	var b strings.Builder
	e.writeScrollbackDelta(&b, vm, sbLen, rows)
	dirty := e.writeScreenDiff(&b, vm, cols, rows)
	e.writeCursorDelta(&b, vm, dirty)

	e.scrollbackLen = sbLen
	e.lastCursor = vm.emu.CursorPosition()
	if b.Len() == 0 {
		return nil, false
	}
	return []byte(b.String()), false
}

// writeScrollbackDelta emits every scrollback line the model committed since
// the last emit. Technique (mirrors the serializer's writeContent flow): park
// the cursor on the bottom row, then write each line followed by CR+LF — the
// client scrolls, the line enters ITS scrollback, and the screen area is
// repainted by the screen diff that follows (which sees all rows dirty after
// a scroll anyway). Primary buffer only; the alt screen has no scrollback.
func (e *DiffEmitter) writeScrollbackDelta(
	b *strings.Builder,
	vm *vtModel,
	sbLen int,
	rows int,
) {
	if e.alt || sbLen <= e.scrollbackLen {
		return
	}
	b.WriteString(cup(rows, 1)) // park on the bottom row
	for y := e.scrollbackLen; y < sbLen; y++ {
		line := vm.emu.ScrollbackLine(y)
		b.WriteString("\r\n")
		b.WriteString(encodeLine(line, len(line), true))
	}
}

// writeScreenDiff rewrites every changed grid row in place (CUP + encoded row,
// pen reset per row via encodeLine's contract) and updates the diff base.
// Returns whether anything was written.
func (e *DiffEmitter) writeScreenDiff(
	b *strings.Builder,
	vm *vtModel,
	cols int,
	rows int,
) bool {
	dirty := false
	for y := 0; y < rows; y++ {
		row := snapshotRow(vm.emu, cols, y)
		if rowsEqual(e.lastGrid[y], row) {
			continue
		}
		dirty = true
		b.WriteString(cup(y+1, 1)) // row y+1, col 1 (1-based)
		b.WriteString(ansi.EraseLineRight)
		b.WriteString(encodeGridRow(vm.emu, cols, y))
		e.lastGrid[y] = row
	}
	return dirty
}

// writeCursorDelta repositions the client cursor to the model's position when
// it moved or when screen rewrites displaced it.
func (e *DiffEmitter) writeCursorDelta(
	b *strings.Builder,
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
	row := make([]uv.Cell, cols)
	for x := 0; x < cols; x++ {
		if c := emu.CellAt(x, y); c != nil {
			row[x] = *c
		} else {
			row[x] = uv.EmptyCell
		}
	}
	return row
}

func rowsEqual(a, b []uv.Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(&b[i]) {
			return false
		}
	}
	return true
}
