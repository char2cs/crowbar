package model

import (
	"image/color"
	"sort"
	"strconv"
	"strings"

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
	lastGrid      [][]uv.Cell
	lastCursor    uv.Position
	chrome        chromeBase
}

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
// guard compares against.
func captureChrome(sh *shadowState) chromeBase {
	modes := make(map[int]bool, len(sh.modes))
	for k, v := range sh.modes {
		modes[k] = v
	}
	return chromeBase{
		modes:           modes,
		title:           sh.title,
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
	e.lastCursor = vm.emu.CursorPosition()
	e.lastGrid = snapshotGrid(vm.emu, e.cols, e.rows)
	e.chrome = captureChrome(&vm.shadow)
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
	e.writeChromeDelta(&b, sh)

	e.scrollbackLen = sbLen
	e.lastCursor = vm.emu.CursorPosition()
	e.chrome = captureChrome(sh)
	if b.Len() == 0 {
		return nil, false
	}
	return []byte(b.String()), false
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
	b *strings.Builder,
	vm *vtModel,
	sbLen int,
	rows int,
) {
	if e.alt || sbLen <= e.scrollbackLen {
		return
	}
	for base := e.scrollbackLen; base < sbLen; base += rows {
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

// writeChromeDelta emits mode flips, title changes, and cursor-chrome changes
// (visibility, style, default colours) since the diff base. Mode order
// follows serializedModeOrder for determinism; modes outside that list still
// stream (sorted numerically) so nothing an app set is silently dropped.
// Mode 6 (DECOM) is excluded: a change to it forces a keyframe in Emit before
// writeChromeDelta ever runs, so forwarding it here too would double-handle
// it once Prime re-captures.
func (e *DiffEmitter) writeChromeDelta(
	b *strings.Builder,
	sh *shadowState,
) {
	e.writeModeDelta(b, sh)
	if sh.title != e.chrome.title {
		b.WriteString("\x1b]0;" + sanitizeOSCText(sh.title) + "\x07")
	}
	e.writeCursorChromeDelta(b, sh)
}

// writeModeDelta diffs the DEC private mode set (excluding mode 6, which is
// handled by Emit's keyframe guard) and forwards each flip as SetMode or
// ResetMode.
func (e *DiffEmitter) writeModeDelta(
	b *strings.Builder,
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
	b *strings.Builder,
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
