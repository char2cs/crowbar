package model

import (
	"bytes"
	"hash/fnv"
	"image/color"
	"sort"
	"strconv"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
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
	lastCursor     uv.Position
	chrome         chromeBase
}

// rotationScanLimit bounds how far back scrollbackNewStart scans for the
// rotation anchor before giving up and demanding a keyframe. A gap larger than
// this between emits is rare (it needs hundreds of lines committed in a single
// coalesce window) and a keyframe resync is the correct, cheap recovery.
const rotationScanLimit = 256

// chromeBase is the subset of shadow state whose CHANGES must stream to live
// clients between keyframes. Grid content is covered by the screen diff; this
// covers everything else a client-side terminal tracks statefully: DEC
// private modes, the window title, cursor visibility/style and the app-set
// default colours. Scroll region and origin mode (DECOM, mode 6) are tracked
// here too, but ONLY to detect their change in Emit's keyframe guard — the
// diff emitter's absolute CUPs are not valid once either is active, so a
// change to either forces a keyframe rather than a diff (see Emit).
type chromeBase struct {
	modes           map[int]bool
	title           string
	iconName        string
	cursorVisible   bool
	cursorShapeSet  bool
	cursorShape     vt.CursorStyle
	cursorBlink     bool
	fg, bg          color.Color
	cursorColor     color.Color
	fgSet, bgSet    bool
	cursorColorSet  bool
	scrollRegionSet bool
	scrollTop       int
	scrollBottom    int
	origin          bool
}

// captureChrome snapshots the shadow fields writeChromeDelta diffs against on
// the NEXT Emit, plus the scroll-region/origin-mode fields Emit's keyframe
// guard compares against. It copies the mode map; Prime (no prior base) uses it.
func captureChrome(sh *shadowState) chromeBase {
	return captureChromeModes(sh, copyModes(sh.modes))
}

// captureChromeModes builds the chrome snapshot using the caller-supplied modes
// map (assumed already owned by the emitter). Splitting the map out lets Emit
// REUSE the previous base's map when the mode set did not change (the common
// echo case) instead of allocating and copying a fresh one every frame — the
// emitter never mutates e.chrome.modes in place, so sharing the unchanged
// reference is safe until the next genuine change re-copies it.
func captureChromeModes(sh *shadowState, modes map[int]bool) chromeBase {
	return chromeBase{
		modes:           modes,
		title:           sh.title,
		iconName:        sh.iconName,
		cursorVisible:   sh.cursorVisible,
		cursorShapeSet:  sh.cursorShapeSet,
		cursorShape:     sh.cursorShape,
		cursorBlink:     sh.cursorBlink,
		fg:              sh.fg,
		bg:              sh.bg,
		cursorColor:     sh.cursorColor,
		fgSet:           sh.fgSet,
		bgSet:           sh.bgSet,
		cursorColorSet:  sh.cursorColorSet,
		scrollRegionSet: sh.scrollRegionSet,
		scrollTop:       sh.scrollTop,
		scrollBottom:    sh.scrollBottom,
		origin:          sh.modes[6],
	}
}

// copyModes returns a fresh copy of a DEC-private-mode map.
func copyModes(src map[int]bool) map[int]bool {
	m := make(map[int]bool, len(src))
	for k, v := range src {
		m[k] = v
	}
	return m
}

// recaptureChrome snapshots chrome for the NEXT Emit, reusing the current base's
// mode map when the shadow's modes are unchanged (skipping the per-frame map
// copy on the hot echo path). Callers that have no prior base (Prime) use
// captureChrome instead.
func (e *DiffEmitter) recaptureChrome(sh *shadowState) chromeBase {
	modes := e.chrome.modes
	if !modesEqual(sh.modes, modes) {
		modes = copyModes(sh.modes)
	}
	return captureChromeModes(sh, modes)
}

// colorsEqual reports whether two color.Color values represent the same RGBA
// value. It is nil-safe: two nil colours are equal, and a nil next to a
// non-nil colour is not.
func colorsEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
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
	if e.scrollbackLen > 0 {
		e.scrollbackTail = scrollbackLineHash(vm, e.scrollbackLen-1)
	} else {
		e.scrollbackTail = 0
	}
	e.lastCursor = vm.emu.CursorPosition()
	e.lastGrid = snapshotGrid(vm.emu, e.cols, e.rows)
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
			e.lastGrid[y] = nil // nil never equals a real row → forced rewrite
		}
	}

	var b bytes.Buffer
	e.writeScrollbackDelta(&b, vm, sbStart, sbLen, rows)
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
	_, _ = h.Write([]byte(encodeLine(line, len(line), true)))
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
func (e *DiffEmitter) writeScrollbackDelta(
	b *bytes.Buffer,
	vm *vtModel,
	start int,
	sbLen int,
	rows int,
) {
	if e.alt || start >= sbLen {
		return
	}
	for base := start; base < sbLen; base += rows {
		batch := sbLen - base
		if batch > rows {
			batch = rows
		}
		for j := 0; j < batch; j++ {
			line := vm.emu.ScrollbackLine(base + j)
			b.WriteString(cup(j+1, 1))
			b.WriteString(ansi.EraseLineRight)
			b.WriteString(encodeLine(line, len(line), true))
		}
		b.WriteString(cup(rows, 1))
		for j := 0; j < batch; j++ {
			b.WriteString("\r\n")
		}
	}
}

// writeScreenDiff rewrites every changed grid row in place (CUP + encoded row,
// pen reset per row via encodeLine's contract) and updates the diff base.
// Returns whether anything was written.
func (e *DiffEmitter) writeScreenDiff(
	b *bytes.Buffer,
	vm *vtModel,
	cols int,
	rows int,
) bool {
	dirty := false
	for y := 0; y < rows; y++ {
		if rowEqualsGrid(vm.emu, e.lastGrid[y], cols, y) {
			continue
		}
		dirty = true
		b.WriteString(cup(y+1, 1)) // row y+1, col 1 (1-based)
		b.WriteString(ansi.EraseLineRight)
		b.WriteString(encodeGridRow(vm.emu, cols, y))
		e.lastGrid[y] = snapshotRow(vm.emu, cols, y)
	}
	return dirty
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

// writeChromeDelta emits mode flips, title/icon-name changes, and cursor-chrome
// changes (visibility, style, default colours) since the diff base. Mode order
// follows serializedModeOrder for determinism; modes outside that list still
// stream (sorted numerically) so nothing an app set is silently dropped.
// Mode 6 (DECOM) is excluded: a change to it forces a keyframe in Emit before
// writeChromeDelta ever runs, so forwarding it here too would double-handle
// it once Prime re-captures.
func (e *DiffEmitter) writeChromeDelta(
	b *bytes.Buffer,
	sh *shadowState,
) {
	e.writeModeDelta(b, sh)
	if sh.title != e.chrome.title {
		b.WriteString("\x1b]0;" + sanitizeOSCText(sh.title) + "\x07")
	}
	// Icon name streams independently of the title: an app can set it alone via
	// OSC 1 (OSC 0 sets both, OSC 2 the title only). Emitted AFTER the title's
	// OSC 0 above so, when a single OSC 0 changed both, this re-asserts the icon
	// the OSC 0 already implied — and when only the icon changed, this is the sole
	// carrier. Uses the serializer's exact OSC 1 ST form (oscTitleSeq).
	if sh.iconName != e.chrome.iconName {
		b.WriteString(oscTitleSeq(1, sanitizeOSCText(sh.iconName)))
	}
	e.writeCursorChromeDelta(b, sh)
}

// writeModeDelta diffs the DEC private mode set (excluding mode 6, which is
// handled by Emit's keyframe guard) and forwards each flip as SetMode or
// ResetMode.
func (e *DiffEmitter) writeModeDelta(
	b *bytes.Buffer,
	sh *shadowState,
) {
	if modesEqual(sh.modes, e.chrome.modes) {
		return
	}
	seen := map[int]bool{6: true}
	keys := make([]int, 0, len(serializedModeOrder)+len(sh.modes)+len(e.chrome.modes))
	for _, k := range serializedModeOrder {
		if seen[k] {
			continue
		}
		keys = append(keys, k)
		seen[k] = true
	}
	extra := make([]int, 0)
	for k := range sh.modes {
		if !seen[k] {
			extra = append(extra, k)
			seen[k] = true
		}
	}
	for k := range e.chrome.modes {
		if !seen[k] {
			extra = append(extra, k)
			seen[k] = true
		}
	}
	sort.Ints(extra)
	keys = append(keys, extra...)

	for _, mode := range keys {
		now, nowOK := sh.modes[mode]
		was, wasOK := e.chrome.modes[mode]
		if nowOK == wasOK && now == was {
			continue
		}
		if now {
			b.WriteString(ansi.SetMode(ansi.DECMode(mode)))
		} else {
			b.WriteString(ansi.ResetMode(ansi.DECMode(mode)))
		}
	}
}

// writeCursorChromeDelta diffs cursor visibility, cursor style/blink and the
// app-set default fg/bg/cursor colours, mirroring the emission forms
// writeCursor/writeChrome use in the serializer's full redraw.
//
// Unlike the serializer — whose keyframe is always preceded by DECSTR, so it
// only ever needs to assert the current ON state — this diff path has no
// implicit reset between emits: every (set bool, value) pair must be handled
// SYMMETRICALLY, emitting the corresponding reset sequence on a set→unset
// transition (e.g. OSC 110/111/112 for the default colours, DECSCUSR 0 for
// cursor shape) just as it emits the set form on a value change. Dropping the
// unset direction would leave a live client stuck on a stale colour/shape
// until an unrelated keyframe happens to resync it.
func (e *DiffEmitter) writeCursorChromeDelta(
	b *bytes.Buffer,
	sh *shadowState,
) {
	if sh.cursorVisible != e.chrome.cursorVisible {
		if sh.cursorVisible {
			b.WriteString(ansi.SetMode(ansi.DECMode(25)))
		} else {
			b.WriteString(ansi.ResetMode(ansi.DECMode(25)))
		}
	}
	switch {
	case sh.cursorShapeSet && (!e.chrome.cursorShapeSet ||
		sh.cursorShape != e.chrome.cursorShape ||
		sh.cursorBlink != e.chrome.cursorBlink):
		b.WriteString(ansi.SetCursorStyle(decscusr(sh.cursorShape, sh.cursorBlink)))
	case !sh.cursorShapeSet && e.chrome.cursorShapeSet:
		b.WriteString(ansi.SetCursorStyle(0)) // DECSCUSR default, undoing the app's explicit style
	}
	switch {
	case sh.fgSet && (!e.chrome.fgSet || !colorsEqual(sh.fg, e.chrome.fg)):
		b.WriteString(oscColor(10, sh.fg))
	case !sh.fgSet && e.chrome.fgSet:
		b.WriteString(ansi.ResetForegroundColor)
	}
	switch {
	case sh.bgSet && (!e.chrome.bgSet || !colorsEqual(sh.bg, e.chrome.bg)):
		b.WriteString(oscColor(11, sh.bg))
	case !sh.bgSet && e.chrome.bgSet:
		b.WriteString(ansi.ResetBackgroundColor)
	}
	switch {
	case sh.cursorColorSet && (!e.chrome.cursorColorSet || !colorsEqual(sh.cursorColor, e.chrome.cursorColor)):
		b.WriteString(oscColor(12, sh.cursorColor))
	case !sh.cursorColorSet && e.chrome.cursorColorSet:
		b.WriteString(ansi.ResetCursorColor)
	}
}

// modesEqual reports whether two mode maps hold identical entries, letting
// writeModeDelta skip the ordered-list allocation on the common no-change
// Emit. A length mismatch short-circuits; otherwise every key in a must be
// present in b with the same value (equal length rules out b having extra
// keys a lacks).
func modesEqual(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
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
