package model

import (
	"bytes"
	"hash/fnv"
	"strconv"

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
	// scrollbackTail is the FNV-1a hash of the LAST scrollback line as of the
	// previous Emit/Prime (zero when scrollbackLen==0). It is the rotation
	// anchor: x/vt's ring evicts the oldest line at cap, so once saturated
	// ScrollbackLen() plateaus and a plain length compare can no longer see
	// scrolled-off lines. Matching this hash back in the current ring (see
	// scrollbackNewStart) recovers the true new-line boundary across rotation.
	scrollbackTail uint64
	lastGrid       [][]uv.Cell
	// lastWrapped is each grid row's wrap bit as the client holds it.
	lastWrapped []bool
	lastCursor  uv.Position
	chrome      chromeBase
}

// rotationScanLimit bounds how far back scrollbackNewStart scans for the
// rotation anchor before giving up and demanding a keyframe. A gap larger than
// this between emits is rare (it needs hundreds of lines committed in a single
// coalesce window) and a keyframe resync is the correct, cheap recovery.
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

// Emit returns the incremental ANSI since the last Prime/Emit, or
// needKeyframe=true when diffing is impossible. This is the ONE canonical,
// exhaustive list of keyframe triggers (other docs reference it rather than
// restate it):
//
//  1. emitter unprimed or Invalidate()d (no valid base)
//  2. grid resize (cols or rows changed)
//  3. alt-screen flip (primary↔alt)
//  4. scrollback shrink (sbLen < the primed length — an ED3-style clear)
//  5. scroll-region (DECSTBM) or origin-mode (DECOM, mode 6) CHANGE vs the base
//     — the diff's absolute CUPs are invalid under either
//  6. rotation-anchor loss — the scrollback tail hashed at the last emit rotated
//     out of the bounded scan window (scrollbackNewStart returned ok=false)
//  7. committed scrollback lines WHILE a scroll region or origin mode is already
//     active (set before the base was primed) — the client's park-at-CUP(rows,1)
//     +LF scroll trick cannot deposit them into history under an active region
//
// On success the emitter re-bases itself.
//
//nolint:gocyclo // Emit is a linear sequence of independent bail-to-keyframe guards (§ ordering); decomposing them would obscure the documented step order without lowering real complexity.
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
		// The diff emitter's absolute CUPs assume no active scroll region and
		// no origin mode; a change to either is only correct to reproduce via
		// a full keyframe redraw (which handles both in its documented step
		// order), never an incremental diff.
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
		// Finding B: an active scroll region or origin mode confines the
		// client's park-at-cup(rows,1)+LF scroll trick — it does not scroll
		// below the region bottom, so committed lines would never enter client
		// history. A keyframe reset clears + re-asserts both, then re-primes.
		// (A change to either is already caught by the guard above; this covers
		// a region/origin set BEFORE Prime, which passes that change-guard.)
		if sh.scrollRegionSet || sh.modes[6] {
			return nil, true
		}
		// The client screen scrolls while absorbing the delta; every row's
		// on-screen identity moves, so rebuild the whole viewport after.
		for y := range e.lastGrid {
			// Truncate instead of nilling: a zero-length row still fails
			// rowEqualsGrid's len(row) != cols check, so the forced rewrite is
			// unchanged, but the backing array survives for snapshotRowInto to
			// refill. Nilling here re-allocated every row on every scroll.
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

// scrollbackNewStart returns the index of the first scrollback line committed
// since the last emit/prime, tolerating x/vt's ring rotation at cap. It anchors
// on e.scrollbackTail (the hash of the previously-last scrollback line): it
// scans BACKWARD from the current tail within rotationScanLimit lines and
// returns the index just past the match. ok=false means the anchor rotated out
// of the scan window — the caller must resync via keyframe. This single
// mechanism covers both plain growth and growth-with-rotation-at-cap: whatever
// lies after the anchor is new.
//
// The scan runs ONLY once the ring has saturated (sbLen >= vm.scrollbackLines).
// Below saturation nothing has ever been evicted, so the previous scrollbackLen
// is already the exact boundary by construction — scanning there would only
// risk misanchoring on a newer line whose CONTENT duplicates the anchor (not
// just a 2^-64 hash collision: repeated blank/prompt lines are common, and if
// the batch just committed itself ends in one, its hash equals the old tail's
// and the scan matches immediately at the new tail, silently discarding the
// whole batch). See scrollbackLineHash for the saturated-path residual this
// still leaves.
//
// Lazy: with no anchor yet (scrollbackLen==0) every current line is new; and
// below saturation the anchor is always still the tail, so the per-emit hash
// is skipped entirely (keeping the interactive echo path hash-free).
func (e *DiffEmitter) scrollbackNewStart(vm *vtModel, sbLen int) (start int, ok bool) {
	if e.alt {
		return sbLen, true // alt screen has no scrollback
	}
	if e.scrollbackLen == 0 {
		return 0, true
	}
	if sbLen < vm.scrollbackLines {
		// Ring never saturated → nothing has ever been evicted → the previous
		// length IS the exact boundary; scanning could only misanchor on
		// duplicate content (e.g. a freshly committed batch that itself ends
		// in a blank line, hashing the same as the old tail). Scan solely at
		// saturation (sbLen == cap), which covers both the grew-into-cap
		// transition and the plateau.
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

// scrollbackLineHash returns an FNV-1a hash over scrollback line idx's encoded
// content — the anchor scrollbackNewStart matches against. Two distinct
// misplacement triggers exist, both scoped to the SATURATED ring (the only
// regime scrollbackNewStart still scans in): a genuine 2^-64 FNV-1a collision,
// and — far more likely in practice — a newer scrolled-off line whose CONTENT
// legitimately duplicates the anchor (repeated blank or prompt lines).
// Either misplaces the new-line boundary (duplicating or dropping a scrolled
// line in client history) and self-heals on the next keyframe — an accepted
// residual.
func scrollbackLineHash(vm *vtModel, idx int) uint64 {
	line := vm.emu.ScrollbackLine(idx)
	h := fnv.New64a()
	h.Write([]byte(encodeLine(line, len(line), true))) //nolint:gosec // G104: a hash.Hash never fails a Write
	return h.Sum64()
}

// EmitterGridBytes returns a coarse resident-size estimate for the diff base a
// DiffEmitter retains for a cols×rows grid (the lastGrid cell buffer), for the
// session's memory accounting (spec §4). ~32B/cell covers each uv.Cell's
// content string header, style and link. Computed from the MODEL's dimensions
// rather than the emitter's primed state so the estimate is stable from spawn:
// the allocation is inevitable for a live session, and lazy accounting would
// otherwise make a session's reported size jump ~3× at its first output —
// destabilizing the engine's memory-ceiling arithmetic between observations.
func EmitterGridBytes(cols, rows int) int64 {
	return int64(cols) * int64(rows) * 32
}

// writeScrollbackDelta replays every scrollback line the model committed since
// the last emit INTO the client's own scrollback. Technique (write-then-scroll,
// chunked by screen height): paint each batch of committed lines onto the top
// rows, then scroll the batch out with LFs from a parked bottom-row cursor —
// each scroll flushes a top row we JUST wrote, so the client's scrollback
// receives exactly the committed content. (Scrolling FIRST would flush the
// client's stale pre-delta rows instead — the bug this replaced.) The screen is
// left trashed by design; Emit invalidates the row cache on any scrollback
// growth, so the screen diff that follows repaints the full viewport. Primary
// buffer only; the alt screen has no scrollback.
//
// A batch ends where a chain does (scrollbackBatch), so the client's autowrap rebuilds each wrap
// bit; a top row continuing the last line is written after it before the scroll, so it stays on
// the client's top row holding its flag.
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
	held, top := e.lastWrapped[0], false
	for base := start; base < sbLen; {
		n := scrollbackBatch(emu, base, sbLen, rows)
		top = base+n == sbLen && n < rows && emu.RowWrapped(0)
		e.writeBatch(b, emu, base, n, cols, held, top)
		b.WriteString(cup(rows, 1))
		for j := 0; j < n; j++ {
			b.WriteString("\r\n")
		}
		base, held = base+n, false
	}
	if wrapOff {
		b.WriteString(ansi.ResetModeAutoWrap)
	}
	e.lastWrapped[0] = top
}

// writeBatch paints committed lines base..base+n-1 from the top row. A line that continues the one
// above it follows that padded head with no address; the first line keeps the flag its client row
// holds only when held says so. With top, the viewport's top row follows the last line.
func (e *DiffEmitter) writeBatch(
	b *bytes.Buffer,
	emu emulator,
	base, n, cols int,
	held, top bool,
) {
	for j := 0; j < n; j++ {
		i := base + j
		wrapped := emu.ScrollbackRowWrapped(i) && (j > 0 || held)
		if j == 0 || !wrapped {
			b.WriteString(cup(j+1, 1))
		}
		if !wrapped {
			b.WriteString(ansi.EraseLineRight)
		}
		next := j+1 < n && emu.ScrollbackRowWrapped(i+1) || j+1 == n && top
		b.Write(appendChainRow(b.AvailableBuffer(), emu.ScrollbackLine(i), cols, wrapped, next, ""))
	}
	if !top {
		return
	}
	row := snapshotRowInto(emu, e.lastGrid[0], cols, 0)
	b.Write(appendChainRow(b.AvailableBuffer(), row, cols, true, false, ""))
	e.lastGrid[0] = row[:0] // still the forced-rewrite marker Emit set
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
		b.Write(appendChainRow(b.AvailableBuffer(), e.lastGrid[y], cols, emu.RowWrapped(y), emu.RowWrapped(y+1), ""))
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

// snapshotRowInto fills dst with row y and returns it, reusing dst's backing
// array when it is large enough. Every element in [0,cols) is overwritten, so a
// reused array carries no stale cells. A nil dst (the scrollback-growth
// invalidation marker set in Emit) allocates, which keeps that marker's
// force-rewrite meaning intact.
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

// rowEqualsGrid reports whether row y of emu matches lastRow without
// allocating a fresh snapshot to compare against. A nil lastRow (the
// scrollback-growth invalidation marker set in Emit) always reports NOT
// equal, forcing the row to be rewritten and re-snapshotted.
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
