package model

import (
	"bytes"
	"image/color"
	"sort"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

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

func captureChrome(sh *shadowState) chromeBase {
	return captureChromeModes(sh, copyModes(sh.modes))
}

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

func copyModes(src map[int]bool) map[int]bool {
	m := make(map[int]bool, len(src))
	for k, v := range src {
		m[k] = v
	}
	return m
}

func (e *DiffEmitter) recaptureChrome(sh *shadowState) chromeBase {
	modes := e.chrome.modes
	if !modesEqual(sh.modes, modes) {
		modes = copyModes(sh.modes)
	}
	return captureChromeModes(sh, modes)
}

func colorsEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func (e *DiffEmitter) writeChromeDelta(
	b *bytes.Buffer,
	sh *shadowState,
) {
	e.writeModeDelta(b, sh)
	if sh.title != e.chrome.title {
		b.WriteString(oscTitleSeq(2, sanitizeOSCText(sh.title)))
	}
	if sh.iconName != e.chrome.iconName {
		b.WriteString(oscTitleSeq(1, sanitizeOSCText(sh.iconName)))
	}
	e.writeCursorChromeDelta(b, sh)
}

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
		b.WriteString(ansi.SetCursorStyle(0))
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

func (e *DiffEmitter) clientAutowrapOff() bool {
	on, ok := e.chrome.modes[7]
	return ok && !on
}

func (e *DiffEmitter) regionEndsInside(
	y0 int,
	end int,
) bool {
	bottom := e.chrome.scrollBottom - 1
	return e.chrome.scrollRegionSet && y0 <= bottom && bottom < end
}
