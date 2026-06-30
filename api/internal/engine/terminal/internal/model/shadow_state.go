package model

import (
	"image/color"

	"github.com/charmbracelet/x/vt"
)

type shadowState struct {
	title           string
	iconName        string
	altScreen       bool
	cursorVisible   bool
	cursorShape     vt.CursorStyle
	cursorBlink     bool
	cursorShapeSet  bool
	scrollRegionSet bool
	scrollTop       int
	scrollBottom    int
	g0              byte
	g1              byte
	glLock          int
	modes           map[int]bool
	fg              color.Color
	bg              color.Color
	cursorColor     color.Color
	fgSet           bool
	bgSet           bool
	cursorColorSet  bool
}

// trackedModes is the set of DEC private modes the serializer re-asserts. Modes outside
// this set are ignored so the map stays small and serialization is minimal.
var trackedModes = map[int]bool{
	1:    true, // DECCKM application cursor keys
	6:    true, // DECOM origin mode
	7:    true, // DECAWM autowrap
	1000: true, // X11 mouse
	1002: true, // button-event mouse
	1003: true, // any-event mouse
	1004: true, // focus reporting
	1006: true, // SGR mouse encoding
	1015: true, // urxvt mouse encoding
	2004: true, // bracketed paste
}

// transientModes are the app-owned modes OnForegroundReset clears. DECAWM (?7) is reset
// separately back to its default-ON state.
var transientModes = []int{1, 6, 1000, 1002, 1003, 1004, 1006, 1015, 2004}

func newShadowState() shadowState {
	return shadowState{
		cursorVisible: true,
		g0:            'B',
		g1:            'B',
		glLock:        0,
		modes:         make(map[int]bool),
	}
}

func (s *shadowState) setMode(
	mode int,
	on bool,
) {
	if !trackedModes[mode] {
		return
	}
	s.modes[mode] = on
}

func (s *shadowState) setCharset(
	slot int,
	designator byte,
) {
	if slot == 0 {
		s.g0 = designator
		return
	}
	s.g1 = designator
}

func (s *shadowState) setDefaultColor(
	slot int,
	c color.Color,
) {
	switch slot {
	case 0:
		s.fg, s.fgSet = c, true
	case 1:
		s.bg, s.bgSet = c, true
	case 2:
		s.cursorColor, s.cursorColorSet = c, true
	}
}

// resetTransientModes clears alt-screen, mouse, focus, bracketed-paste, app-cursor-keys,
// origin, charset designation, scroll region and cursor shape, and restores autowrap to
// its default-ON state. Title and the scrollback are intentionally preserved.
func (s *shadowState) resetTransientModes() {
	s.altScreen = false
	s.cursorShapeSet = false
	s.cursorShape = vt.CursorBlock
	s.cursorBlink = false
	s.g0, s.g1, s.glLock = 'B', 'B', 0
	s.scrollRegionSet = false
	s.scrollTop, s.scrollBottom = 0, 0
	for _, m := range transientModes {
		delete(s.modes, m)
	}
	delete(s.modes, 7)
}
