package model

import (
	"image/color"
	"log/slog"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

// bytesPerCell is the conservative per-cell resident-size estimate (rune + style +
// link) used by ModelBytes for the engine's memory-ceiling accounting.
const bytesPerCell = 16

// foregroundResetTeardown drives x/vt out of any alt buffer (all three exits, since ?1049l alone does not
// leave one entered via ?47h/?1047h) and resets charsets and keypad, without touching the grid. RIS is
// absent on purpose: it would wipe the shell screen and scrollback on every app-death edge.
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

	// Scanner state (escan.go) for SCS, SI/SO, DECSTBM and OSC titles, which the pinned x/vt does not surface
	// (or truncates at an embedded 0x9C); carried across Writes so a split sequence is still recognised.
	escanState   int
	escanParams  []byte
	escanPrivate bool
	escanOSC     []byte

	// OSC 0/1/2 strip state (osc_strip.go): x/vt ends an OSC at an embedded 0x9C and prints the rest of a
	// UTF-8 title into its grid, so titles are stripped before x/vt; carried across Writes for split OSCs.
	stripState    int
	stripWithheld []byte
	stripCode     []byte

	// responseSink is retained so recreateEmu re-arms it after a parse panic instead of silently
	// discarding device-query replies.
	responseSink func(p []byte)

	// themeBg / themeFg answer OSC 11 / OSC 10 queries; kept on the model so recreateEmu re-applies them
	// over x/vt's hardcoded black. Nil means never set.
	themeBg, themeFg color.Color

	// themeNotify mirrors DEC mode 2031. Not parser state: it survives recreateEmu (the app is still
	// subscribed) but clears on the app-death edge so a shell never gets a CSI ?997;n report.
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

// buildEmu constructs an emulator bound to m; it is the sole construction site so callback wiring lives
// in one place.
func (m *vtModel) buildEmu(
	cols int,
	rows int,
) emulator {
	// Title / IconName are NOT bound: x/vt truncates a UTF-8 title at an embedded 0x9C, so the escan scanner
	// is the authoritative title source.
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

// decModeNumericKeypad is DECMode(66): x/vt routes both CSI ?66 h/l and ESC = / ESC > through it, so it
// is the single application-keypad signal (enabled == application keypad).
const decModeNumericKeypad = 66

// decModeThemeNotify is DEC mode 2031, an app's request for CSI ?997;n theme-change reports. Not a
// rendering mode, so it is kept out of the tracked-mode map in its own flag.
const decModeThemeNotify = 2031

// observeMode records a DEC private mode toggle (ANSI modes are not re-asserted). Mode 66 gets its own
// shadow flag because it must be re-emitted as ESC = / ESC >, which a ?66h does not reproduce.
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

// observeDefaultColor records an app-issued OSC 10/11/12 default colour. A nil colour
// means the app reset to the terminal default, which clears the corresponding set flag
// so the serializer does not override the client theme.
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

// SetDefaultColors sets the colours OSC 11 / OSC 10 queries answer with, so a querying app detects the
// theme. It never sets shadow.bgSet/fgSet, so no OSC SET reaches the transparent client; a nil channel is
// a no-op. Retained for recreateEmu. Caller holds s.mu.
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

// ThemeNotifyEnabled reports whether the foreground app subscribed to theme-change
// notifications via DEC private mode 2031. Part of the ThemeAware surface.
func (m *vtModel) ThemeNotifyEnabled() bool {
	return m.themeNotify
}

// applyThemeColorsLocked re-applies the retained default colours to the current emulator.
// Called from recreateEmu so a parse-panic-recovered (blank, default-black) emulator still
// answers OSC 11/10 queries with the host theme.
func (m *vtModel) applyThemeColorsLocked() {
	if m.themeBg != nil {
		m.emu.SetDefaultBackgroundColor(m.themeBg)
	}
	if m.themeFg != nil {
		m.emu.SetDefaultForegroundColor(m.themeFg)
	}
}

// Write feeds a chunk to the emulator: escan sees the RAW chunk (title capture) while x/vt gets it with
// OSC titles stripped. A parse panic recreates a blank emulator at the current size; the scrollback
// depth and title survive, the grid and history are lost until the app repaints (accepted, self-healing).
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

// recreateEmu replaces the emulator with a blank one after a recovered parse panic and resets transient
// shadow modes, keeping the title (not parser state).
func (m *vtModel) recreateEmu(
	cols int,
	rows int,
) {
	m.emu.Close()
	m.emu = m.buildEmu(cols, rows)
	if m.responseSink != nil {
		m.emu.SetResponseSink(m.responseSink)
	}
	// The fresh emulator resets to black; themeNotify stays because the app is still subscribed.
	m.applyThemeColorsLocked()
	m.shadow.resetTransientModes()
	m.resetEscan()
	m.resetStrip()
}

// SetResponseSink installs the device-query reply receiver, retained so recreateEmu re-arms it.
func (m *vtModel) SetResponseSink(f func(p []byte)) {
	m.responseSink = f
	m.emu.SetResponseSink(f)
}

// ResponseSinkInstalledForTest reports whether m has a response sink, for the session package's
// degraded-flip test; a TerminalModel that is not a *vtModel reports false.
func ResponseSinkInstalledForTest(m TerminalModel) bool {
	vm, ok := m.(*vtModel)
	if !ok {
		return false
	}
	return vm.responseSink != nil
}

// Resize forwards the geometry and clears the shadow scroll region: x/vt resets its region on resize,
// which escan never sees, and a stale DECSTBM re-emitted on re-attach lands the app's repaint a row off.
func (m *vtModel) Resize(
	cols int,
	rows int,
) {
	m.emu.Resize(cols, rows)
	m.shadow.scrollRegionSet = false
	m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
}

// OnForegroundReset clears app-owned state on the foreground-app-death edge, keeping grid, scrollback,
// title and cursor. The teardown goes through emu.Write (not m.Write, no escan) so x/vt itself leaves the
// alt buffer the serializer reads; idempotent. Not goroutine-safe: the session's lock guards it.
func (m *vtModel) OnForegroundReset() {
	m.emu.Write([]byte(foregroundResetTeardown))
	m.shadow.resetTransientModes()
	// Drop the dead app's 2031 subscription so a theme switch never types into the shell; the default
	// colours belong to the host terminal and stay.
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

// ScreenText reads the emulator's own grid (no ANSI re-parse). Never-painted cells render as spaces,
// trailing blanks are trimmed and rows joined with newlines.
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

func (m *vtModel) HeaderState() (cols, rows int, alt bool, scrollbackLines int) {
	return m.emu.Width(), m.emu.Height(), m.emu.IsAltScreen(), m.scrollbackLines
}

// ModelBytes estimates the model's resident size: the live grid plus the emulator's
// retained scrollback cells, at bytesPerCell each. The middle "retained cell window"
// term of §9.4 is the scrollback x/vt itself holds at this pin.
func (m *vtModel) ModelBytes() int64 {
	cols, rows := m.emu.Width(), m.emu.Height()
	sb := m.emu.ScrollbackLen()
	return int64((cols*rows + sb*cols) * bytesPerCell)
}

// reconcileAltScreen reconciles a drifted shadow alt-screen flag to emu.IsAltScreen(), latches the sticky
// degraded flag and warns once per session.
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
