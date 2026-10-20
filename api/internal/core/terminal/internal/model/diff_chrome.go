package model

import (
	"bytes"
	"image/color"
	"sort"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

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
	// Title-only: OSC 2, never OSC 0. OSC 0 sets BOTH the title and the icon
	// name, so emitting it here for a title-only change would silently
	// overwrite the client's icon with the (unchanged) title text — and since
	// shadow.iconName itself didn't change, the icon-name branch below would
	// never fire to correct it, leaving the client's icon wrong until the next
	// keyframe. Uses the serializer's exact OSC 2 ST form (oscTitleSeq) so the
	// keyframe and diff paths emit byte-identical title sequences.
	if sh.title != e.chrome.title {
		b.WriteString(oscTitleSeq(2, sanitizeOSCText(sh.title)))
	}
	// Icon name streams independently of the title: an app can set it alone via
	// OSC 1 (OSC 0 sets both, OSC 2 the title only). Uses the serializer's exact
	// OSC 1 ST form (oscTitleSeq); this is now the sole carrier for an icon
	// change regardless of whether the title also changed, since the title
	// delta above never touches the icon.
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
