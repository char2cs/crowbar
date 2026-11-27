package model

import (
	"image/color"
	"log/slog"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

const bytesPerCell = 16

const foregroundResetTeardown = "\x1b[?1049l\x1b[?47l\x1b[?1047l" +
	"\x1b[?1l\x1b[?6l" +
	"\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1006l\x1b[?1015l" +
	"\x1b[?7h\x1b[?25h\x1b[?2004l\x1b>" +
	"\x1b(B\x1b)B\x0f"

type vtModel struct {
	emu             emulator
	shadow          shadowState
	scrollbackLines int
	degraded        bool
	parsePanics     int
	altDesyncWarned bool

	escanState   int
	escanParams  []byte
	escanPrivate bool
	escanOSC     []byte

	stripState    int
	stripWithheld []byte
	stripCode     []byte

	responseSink func(p []byte)

	themeBg, themeFg color.Color

	themeNotify bool
}

var (
	_ TerminalModel = (*vtModel)(nil)
	_ ModelHealth   = (*vtModel)(nil)
)

func newVTModel(
	cols int,
	rows int,
	scrollbackLines int,
) *vtModel {
	m := &vtModel{
		shadow:          newShadowState(),
		scrollbackLines: scrollbackLines,
	}
	m.emu = m.buildEmu(cols, rows)
	return m
}

func (m *vtModel) buildEmu(
	cols int,
	rows int,
) emulator {
	cb := vt.Callbacks{
		AltScreen:        func(on bool) { m.shadow.altScreen = on },
		CursorVisibility: func(v bool) { m.shadow.cursorVisible = v },
		CursorStyle: func(style vt.CursorStyle, blink bool) {
			m.shadow.cursorShape, m.shadow.cursorBlink = style, blink
			m.shadow.cursorShapeSet = true
		},
		EnableMode:      func(mode ansi.Mode) { m.observeMode(mode, true) },
		DisableMode:     func(mode ansi.Mode) { m.observeMode(mode, false) },
		ForegroundColor: func(c color.Color) { m.observeDefaultColor(0, c) },
		BackgroundColor: func(c color.Color) { m.observeDefaultColor(1, c) },
		CursorColor:     func(c color.Color) { m.observeDefaultColor(2, c) },
	}
	return newEmulator(cols, rows, cb, m.scrollbackLines)
}

const decModeNumericKeypad = 66

const decModeThemeNotify = 2031

func (m *vtModel) observeMode(
	mode ansi.Mode,
	on bool,
) {
	dm, ok := mode.(ansi.DECMode)
	if !ok {
		return
	}
	if dm.Mode() == decModeNumericKeypad {
		m.shadow.keypadApplication = on
		return
	}
	if dm.Mode() == decModeThemeNotify {
		m.themeNotify = on
		return
	}
	m.shadow.setMode(dm.Mode(), on)
}

func (m *vtModel) observeDefaultColor(
	slot int,
	c color.Color,
) {
	if c == nil {
		m.clearDefaultColor(slot)
		return
	}
	m.shadow.setDefaultColor(slot, c)
}

func (m *vtModel) clearDefaultColor(
	slot int,
) {
	switch slot {
	case 0:
		m.shadow.fg, m.shadow.fgSet = nil, false
	case 1:
		m.shadow.bg, m.shadow.bgSet = nil, false
	case 2:
		m.shadow.cursorColor, m.shadow.cursorColorSet = nil, false
	}
}

func (m *vtModel) SetDefaultColors(bg, fg color.Color) {
	if bg != nil {
		m.themeBg = bg
		m.emu.SetDefaultBackgroundColor(bg)
	}
	if fg != nil {
		m.themeFg = fg
		m.emu.SetDefaultForegroundColor(fg)
	}
}

func (m *vtModel) ThemeNotifyEnabled() bool {
	return m.themeNotify
}

func (m *vtModel) applyThemeColorsLocked() {
	if m.themeBg != nil {
		m.emu.SetDefaultBackgroundColor(m.themeBg)
	}
	if m.themeFg != nil {
		m.emu.SetDefaultForegroundColor(m.themeFg)
	}
}

func (m *vtModel) Write(
	p []byte,
) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		c, rr := m.emu.Width(), m.emu.Height()
		m.recreateEmu(c, rr)
		m.degraded = true
		m.parsePanics++
	}()
	m.emu.Write(m.stripOSCTitles(p))
	m.scanCharsetAndRegion(p)
}

func (m *vtModel) recreateEmu(
	cols int,
	rows int,
) {
	m.emu.Close()
	m.emu = m.buildEmu(cols, rows)
	if m.responseSink != nil {
		m.emu.SetResponseSink(m.responseSink)
	}
	m.applyThemeColorsLocked()
	m.shadow.resetTransientModes()
	m.resetEscan()
	m.resetStrip()
}

func (m *vtModel) SetResponseSink(f func(p []byte)) {
	m.responseSink = f
	m.emu.SetResponseSink(f)
}

func ResponseSinkInstalledForTest(m TerminalModel) bool {
	vm, ok := m.(*vtModel)
	if !ok {
		return false
	}
	return vm.responseSink != nil
}

func (m *vtModel) Resize(
	cols int,
	rows int,
) {
	m.emu.Resize(cols, rows)
	m.shadow.scrollRegionSet = false
	m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
}

func (m *vtModel) OnForegroundReset() {
	m.emu.Write([]byte(foregroundResetTeardown))
	m.shadow.resetTransientModes()
	m.themeNotify = false
}

func (m *vtModel) Title() string {
	return m.shadow.title
}

func (m *vtModel) Cols() int {
	return m.emu.Width()
}

func (m *vtModel) Rows() int {
	return m.emu.Height()
}

func (m *vtModel) ScreenText() string {
	cols, rows := m.emu.Width(), m.emu.Height()
	var out strings.Builder
	out.Grow(cols * rows)
	var line strings.Builder
	line.Grow(cols)
	for y := range rows {
		line.Reset()
		for x := range cols {
			if cell := m.emu.CellAt(x, y); cell != nil {
				line.WriteString(cell.String())
			} else {
				line.WriteByte(' ')
			}
		}
		if y > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(strings.TrimRight(line.String(), " "))
	}
	return out.String()
}

const decModeBracketedPaste = 2004

func (m *vtModel) ScreenState() ScreenState {
	pos := m.emu.CursorPosition()
	return ScreenState{
		Text:           m.ScreenText(),
		CursorRow:      pos.Y,
		CursorCol:      pos.X,
		Faint:          m.faintCells(),
		BracketedPaste: m.shadow.modes[decModeBracketedPaste],
	}
}

func (m *vtModel) faintCells() [][]bool {
	cols, rows := m.emu.Width(), m.emu.Height()
	faint := make([][]bool, rows)
	for y := range rows {
		faint[y] = make([]bool, cols)
		for x := range cols {
			if cell := m.emu.CellAt(x, y); cell != nil {
				faint[y][x] = cell.Style.Attrs&uv.AttrFaint != 0
			}
		}
	}
	return faint
}

func (m *vtModel) HeaderState() (cols, rows int, alt bool, scrollbackLines int) {
	return m.emu.Width(), m.emu.Height(), m.emu.IsAltScreen(), m.scrollbackLines
}

func (m *vtModel) ModelBytes() int64 {
	cols, rows := m.emu.Width(), m.emu.Height()
	sb := m.emu.ScrollbackLen()
	return int64((cols*rows + sb*cols) * bytesPerCell)
}

func (m *vtModel) reconcileAltScreen(
	alt bool,
) {
	if m.shadow.altScreen == alt {
		return
	}
	m.shadow.altScreen = alt
	m.degraded = true
	if m.altDesyncWarned {
		return
	}
	m.altDesyncWarned = true
	slog.Warn("terminal: alt-screen shadow desync; reconciled to emulator", "component", "terminal", "alt", alt)
}

func (m *vtModel) Close() {
	m.emu.Close()
	m.shadow = newShadowState()
}

func (m *vtModel) Degraded() bool {
	return m.degraded
}

func (m *vtModel) ParsePanics() int {
	return m.parsePanics
}
