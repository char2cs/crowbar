package model

import (
	"bytes"
	"image/color"
	"sort"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// chromeBase is the shadow state whose CHANGES stream between keyframes (modes, title, cursor, default
// colours); the screen diff covers the grid. Scroll region and DECOM are kept too: changing either forces
// a keyframe, since the diff's absolute CUPs assume them fixed.
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

// captureChromeModes builds the chrome snapshot over a caller-owned modes map, so Emit can reuse the
// previous map when modes are unchanged; the emitter never mutates it in place, so sharing is safe.
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

// recaptureChrome snapshots chrome for the NEXT Emit, reusing the mode map when unchanged; Prime, with no
// prior base, uses captureChrome.
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

// writeChromeDelta emits mode, title/icon and cursor-chrome changes since the base, modes in
// serializedModeOrder then numerically so none is dropped. DECOM is excluded: its change already forced a
// keyframe.
func (e *DiffEmitter) writeChromeDelta(
	b *bytes.Buffer,
	sh *shadowState,
) {
	e.writeModeDelta(b, sh)
	// Title-only: OSC 2, never OSC 0, which would also overwrite the client's icon name with no icon delta to
	// correct it. oscTitleSeq keeps diff and keyframe bytes identical.
	if sh.title != e.chrome.title {
		b.WriteString(oscTitleSeq(2, sanitizeOSCText(sh.title)))
	}
	// The icon name streams on its own (OSC 1 can set it alone); this is its sole carrier.
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

// writeCursorChromeDelta diffs cursor visibility/style and default colours. Unlike a keyframe (preceded
// by DECSTR) there is no implicit reset between diffs, so each set->unset transition must emit its reset
// (OSC 110/111/112, DECSCUSR 0) or the client stays stale.
//
//nolint:gocyclo // Four independent, symmetric set/unset emission blocks (visibility, shape, fg/bg/cursor colour); the count is width, not depth, and splitting the mirrored pairs apart would hide the symmetry the doc comment relies on.
func (e *DiffEmitter) writeCursorChromeDelta(
	b *bytes.Buffer,
	sh *shadowState,
) {
	if sh.cursorVisible != e.chrome.cursorVisible { //nolint:nestif // one shallow visibility set/unset branch; flattening it buys nothing.
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

// modesEqual reports whether two mode maps hold identical entries, so writeModeDelta skips allocating
// on the common no-change Emit.
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

// clientAutowrapOff reports DECAWM as the client last received it (absent is on): the rows are
// written before the chrome delta that may change it, and a chain needs autowrap to flow.
func (e *DiffEmitter) clientAutowrapOff() bool {
	on, ok := e.chrome.modes[7]
	return ok && !on
}

// regionEndsInside reports whether the client's scroll region ends above the last of rows y0..end:
// autowrap off its bottom margin scrolls the region instead of moving down, so a chain's flow
// lifts the region (DECSTBM homes the cursor, which the cursor delta re-addresses).
func (e *DiffEmitter) regionEndsInside(
	y0 int,
	end int,
) bool {
	bottom := e.chrome.scrollBottom - 1
	return e.chrome.scrollRegionSet && y0 <= bottom && bottom < end
}
