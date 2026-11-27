package model

import (
	"image/color"

	"github.com/charmbracelet/x/vt"
)

type shadowState struct {
	title             string
	iconName          string
	altScreen         bool
	cursorVisible     bool
	cursorShape       vt.CursorStyle
	cursorBlink       bool
	cursorShapeSet    bool
	scrollRegionSet   bool
	scrollTop         int
	scrollBottom      int
	g0                byte
	g1                byte
	glLock            int
	keypadApplication bool
	modes             map[int]bool
	fg                color.Color
	bg                color.Color
	cursorColor       color.Color
	fgSet             bool
	bgSet             bool
	cursorColorSet    bool
}

var trackedModes = map[int]bool{
	1:    true,
	6:    true,
	7:    true,
	1000: true,
	1002: true,
	1003: true,
	1004: true,
	1006: true,
	1015: true,
	2004: true,
}

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

func (s *shadowState) resetTransientModes() {
	s.altScreen = false
	s.cursorShapeSet = false
	s.cursorShape = vt.CursorBlock
	s.cursorBlink = false
	s.g0, s.g1, s.glLock = 'B', 'B', 0
	s.scrollRegionSet = false
	s.scrollTop, s.scrollBottom = 0, 0
	s.keypadApplication = false
	for _, m := range transientModes {
		delete(s.modes, m)
	}
	delete(s.modes, 7)
}
