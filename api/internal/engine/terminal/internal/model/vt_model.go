package model

import (
	"image/color"
	"log"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

const maxPendingPartial = 4096

// bytesPerCell is the conservative per-cell resident-size estimate (rune + style +
// link) used by ModelBytes for the engine's memory-ceiling accounting.
const bytesPerCell = 16

// foregroundResetTeardown is the synthetic, canonical, NON-DESTRUCTIVE app-teardown
// sequence OnForegroundReset feeds into the emulator on the foreground-app-death edge
// (§11.1). It drives x/vt itself out of whichever alt buffer an app entered and clears
// the app-owned transient modes, so the emulator (the single source of truth the
// serializer reads via emu.IsAltScreen()) and the shadow agree on a clean primary screen.
//
// All three alt-screen exits (?1049l, ?47l, ?1047l) are emitted because an app may have
// entered the alt buffer via the legacy ?47h/?1047h, and ?1049l alone does NOT take x/vt
// out of a buffer entered that way. The trailing ESC(B ESC)B re-designate G0/G1 to
// US-ASCII and SI re-invokes G0 into GL, resetting charset/locking-shift state without
// touching the grid. ESC > (DECKPNM) returns the keypad to numeric mode, undoing any
// application-keypad (ESC =) the dead app left on.
//
// RIS (ESC c) is deliberately ABSENT: RIS clears the grid AND scrollback, which would wipe
// the shell screen on every app-death edge — the opposite of this method's contract. The
// grid, scrollback, title and cursor position are legitimately still on screen at the
// prompt and must be preserved.
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
	pendingPartial  []byte

	// In-Write SCS / SI-SO / DECSTBM / OSC-title scanner state (escan.go). The pinned x/vt
	// commit surfaces none of charset, locking-shift, or scroll-region through vt.Callbacks
	// (so the adapter parses those to keep shadow.g0/g1/glLock and the scroll region populated
	// for serialize steps 10-11), and its OSC callbacks truncate UTF-8 titles at an embedded
	// 0x9C ST byte (so the adapter parses OSC 0/1/2 itself, UTF-8-transparently, for
	// shadow.title / shadow.iconName). State is carried across Write calls so a sequence split
	// across PTY chunks is still recognised.
	escanState   int
	escanParams  []byte
	escanPrivate bool
	escanOSC     []byte

	// OSC 0/1/2 strip-from-emulator state (osc_strip.go). The pinned x/vt honours an embedded
	// 8-bit ST (0x9C) — a UTF-8 continuation byte — as an OSC terminator, so a UTF-8 title
	// carrying that byte makes x/vt drop out of the OSC early and PRINT the leftover title
	// bytes into its grid. We already capture titles UTF-8-transparently via escan, so OSC
	// 0/1/2 are removed from the byte stream before it reaches x/vt. State is carried across
	// Write calls so an OSC split across PTY chunks is stripped correctly and no dangling
	// partial title ever reaches the emulator.
	stripState    int
	stripWithheld []byte
	stripCode     []byte

	// responseSink is the last sink installed via SetResponseSink, retained so
	// recreateEmu (parse-panic recovery, §Write) can re-arm it on the fresh
	// emulator — otherwise a panic recovery would silently revert a model-driven
	// session to discarding device-query replies until the next explicit call.
	responseSink func(p []byte)
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

// buildEmu constructs an emulator of cols x rows with the callbacks bound to m, via the
// newEmulator var. It is the sole emulator-construction site (newVTModel and the
// parse-panic recovery recreateEmu both call it), so the callback wiring exists in
// exactly one place.
func (m *vtModel) buildEmu(
	cols int,
	rows int,
) emulator {
	// Title / IconName are deliberately NOT bound: x/vt's OSC-string parser terminates a
	// UTF-8 title at an embedded 8-bit ST byte (0x9C), which is also a UTF-8 continuation byte,
	// so its callback delivers a truncated, invalid-UTF-8 fragment. The UTF-8-transparent
	// escan scanner (escan.go) is the authoritative source for shadow.title / shadow.iconName.
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

// decModeNumericKeypad is DECMode(66) (ansi.ModeNumericKeypad / DECNKM). x/vt routes both
// the DEC private-mode form (CSI ?66 h/l) AND the two-byte DECKPAM/DECKPNM escapes
// (ESC = / ESC >) through this mode toggle, so it is the single signal for application-
// keypad state. EnableMode(66) == application keypad on (ESC =), DisableMode(66) == numeric
// keypad (ESC >).
const decModeNumericKeypad = 66

// observeMode records a DEC private mode toggle. Non-DEC (ANSI) modes are ignored: only
// private modes are re-asserted by the serializer. Application-keypad mode (DEC 66) is
// pulled out into its own shadow flag because the serializer must re-emit it as ESC = /
// ESC >, not as a DECSET — a plain ?66h would not re-establish application keypad on the
// live xterm client.
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

// Write feeds a chunk to the emulator. The chunk is first passed through stripOSCTitles
// (osc_strip.go), which removes OSC 0/1/2 (icon/title) sequences so x/vt never mis-prints a
// UTF-8 title carrying an embedded 0x9C into its grid; the title is still captured, whole and
// UTF-8-transparently, by scanCharsetAndRegion below. scanCharsetAndRegion and
// trackPendingPartial run over the RAW chunk (title capture and partial-input framing must see
// the unstripped stream); only the EMULATOR is fed the stripped bytes.
//
// On a parse panic — x/vt is an untagged emulator
// fed arbitrary PTY bytes and buffers parser state across writes — the emulator is
// recreated to a known-blank state at the current size so every later Write parses
// correctly. The running app repaints on its next frame and live clients already received
// the raw bytes verbatim.
//
// What survives the recreate: the configured scrollback DEPTH (the SetScrollbackSize
// capacity, re-applied by buildEmu) and the title (it is shadow state, not parser state).
// What is LOST: the visible grid AND the scrollback CONTENT (history). At this pin
// scrollback is sourced entirely from the x/vt emulator's own buffer (D1: the compact-ring
// mechanism that would let history outlive the emulator is deferred), so building a fresh
// emulator discards all accumulated scrollback lines. A client that attaches in the window
// between the panic and the app's next repaint sees a blank screen with no history. This is
// an accepted, self-healing degraded-path residual, recorded in UPSTREAM.md alongside D1 and
// pinned by TestWriteRecoverDiscardsScrollbackContent.
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
		m.pendingPartial = nil
	}()
	m.emu.Write(m.stripOSCTitles(p))
	m.scanCharsetAndRegion(p)
	m.trackPendingPartial(p)
}

// recreateEmu replaces the emulator with a fresh, blank one at cols x rows after a
// recovered parse panic and resets the transient shadow modes to hardware defaults, so
// the emulator and the shadow agree on a clean primary blank screen. Title is preserved
// (it is not parser state).
func (m *vtModel) recreateEmu(
	cols int,
	rows int,
) {
	m.emu.Close()
	m.emu = m.buildEmu(cols, rows)
	if m.responseSink != nil {
		m.emu.SetResponseSink(m.responseSink)
	}
	m.shadow.resetTransientModes()
	m.resetEscan()
	m.resetStrip()
}

// SetResponseSink installs the receiver for the emulator's device-query replies (spec
// §3.8) and forwards it to the current emulator. The sink is retained on the model (not
// just the emulator) so a parse-panic recreate (recreateEmu) re-arms it on the fresh
// emulator rather than silently reverting to discard.
func (m *vtModel) SetResponseSink(f func(p []byte)) {
	m.responseSink = f
	m.emu.SetResponseSink(f)
}

// Resize forwards the new geometry to the emulator and clears the shadow scroll region.
// x/vt's Screen.Resize unconditionally resets its scroll region to the full new bounds
// (s.scroll = buf.Bounds()), and the scroll region is tracked in the shadow by the
// escan DECSTBM scanner — which never observes that emulator-internal reset. Without
// clearing it here, a re-attach Serialize after a SIGWINCH (every workspace switch resizes
// the recreated xterm) would re-emit a STALE DECSTBM the live app has already lost, landing
// the app's next repaint one row off and leaking adjacent chrome into the input line. The
// app re-establishes its real region on its post-resize repaint; until then full-screen
// (no DECSTBM) is the correct, x/vt-matching state.
func (m *vtModel) Resize(
	cols int,
	rows int,
) {
	m.emu.Resize(cols, rows)
	m.shadow.scrollRegionSet = false
	m.shadow.scrollTop, m.shadow.scrollBottom = 0, 0
}

// OnForegroundReset clears transient, app-owned screen state on the foreground-app-death
// edge (§11.1) WITHOUT touching the grid, scrollback, title or cursor position.
//
// It feeds the synthetic foregroundResetTeardown sequence straight into the emulator's
// Write path so x/vt itself leaves whichever alt buffer the (now-dead) app entered and
// clears its modes, with the vt.Callbacks updating the shadow in step. This is the
// load-bearing half: the serializer reads emu.IsAltScreen() (the §4 single source of
// truth), so clearing only the shadow flag would leave x/vt physically in the alt buffer
// and a later Serialize would re-emit ?1049h plus the stale alt grid into the idle prompt.
//
// The teardown is fed via m.emu.Write (NOT m.Write): it must not be tracked as pending
// partial input nor re-scanned by the charset/region escan — it is a model-internal
// correction, never raw PTY output. resetTransientModes() is kept as a belt-and-braces
// guard against a missing callback. Both halves are idempotent, so firing on every
// app->shell edge (including a clean exit that already emitted its own resets) is safe.
//
// Not goroutine-safe: the session's lock guards this. An emulator panic here propagates to
// the session's mutateModelLocked recover backstop (§8.5); the model adds no recover of its
// own, mirroring the Resize path.
func (m *vtModel) OnForegroundReset() {
	m.emu.Write([]byte(foregroundResetTeardown))
	m.shadow.resetTransientModes()
}

func (m *vtModel) PendingInput() []byte {
	return m.pendingPartial
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

func (m *vtModel) HeaderState() (cols int, rows int, alt bool, scrollbackLines int) {
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

// reconcileAltScreen implements the §4 "Mismatch handling" cross-check: the serializer
// passes the authoritative emu.IsAltScreen() value; when the shadow's mirror of it has
// drifted (a missed AltScreen callback), reconcile the shadow to the emulator, latch the
// sticky degraded flag, and warn-log exactly once per session.
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
	log.Printf("terminal model: alt-screen shadow desync (emu=%v); reconciled to emulator", alt)
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

// trackPendingPartial maintains the raw bytes of the incomplete escape/control sequence
// at the end of the cumulative stream, for the mid-sequence attach re-sync. It seeds a
// minimal ECMA-48 escape-framing scanner with the carried partial bytes and the new
// chunk, then keeps only the trailing in-flight sequence (nil in ground state). A
// still-incomplete sequence longer than maxPendingPartial is dropped (the accepted,
// self-healing residual).
//
// The retained tail is required to START WITH ESC (0x1B): scanPartial is UTF-8-transparent
// and its sole introducer is the 7-bit ESC, so a genuine partial always leads with ESC.
// This final guard is defence-in-depth — it guarantees Attach can never concatenate a
// printable-leading multi-command run onto the clean serialize even if the scanner framing
// were to regress. A tail that fails it is dropped (treated as ground); the app's next
// repaint re-syncs the client.
func (m *vtModel) trackPendingPartial(
	p []byte,
) {
	combined := p
	if len(m.pendingPartial) > 0 {
		combined = make([]byte, 0, len(m.pendingPartial)+len(p))
		combined = append(combined, m.pendingPartial...)
		combined = append(combined, p...)
	}
	tail := scanPartial(combined)
	if len(tail) == 0 || len(tail) > maxPendingPartial || tail[0] != 0x1b {
		m.pendingPartial = nil
		return
	}
	m.pendingPartial = append([]byte(nil), tail...)
}
