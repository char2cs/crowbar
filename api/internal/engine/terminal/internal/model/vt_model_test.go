package model

import (
	"bytes"
	"image/color"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
)

// TestEmulatorDrainsDeviceQueryReplies proves the production emulator does not deadlock on a
// device query. The pinned x/vt answers DA/DSR/colour-OSC queries by writing the reply into
// an UNBUFFERED InputPipe; without the drainResponses reader that write blocks inside
// model.Write forever (and, in the session, under the session lock — hanging every later
// Attach). Because the pipe is unbuffered, this Write returns ONLY after the drain goroutine
// has consumed the reply, so a non-blocking return is a direct witness that the drain ran.
func TestEmulatorDrainsDeviceQueryReplies(t *testing.T) {
	m := newVTModel(80, 24, 100)
	done := make(chan struct{})
	go func() {
		// Primary DA + Cursor-Position (DSR) + foreground-colour OSC query — each makes
		// x/vt write a reply into the InputPipe.
		m.Write([]byte("\x1b[c\x1b[6n\x1b]10;?\x1b\\"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("model.Write blocked: x/vt device-query replies were not drained")
	}
	if m.Degraded() {
		t.Fatal("draining a device query must not degrade the model")
	}
	m.Close()
}

// fakeEmu is a stand-in emulator used only to drive the parse-panic recover branch in
// vtModel.Write: its Write panics on a sentinel byte that real PTY bytes are not
// guaranteed to produce.
type fakeEmu struct {
	cols       int
	rows       int
	panicByte  byte
	written    []byte
	scrollback []uv.Line
	closed     int
}

func (f *fakeEmu) Write(p []byte) {
	if bytes.IndexByte(p, f.panicByte) >= 0 {
		panic("sentinel parse panic")
	}
	f.written = append(f.written, p...)
}

func (f *fakeEmu) Resize(cols, rows int)       { f.cols, f.rows = cols, rows }
func (f *fakeEmu) Width() int                  { return f.cols }
func (f *fakeEmu) Height() int                 { return f.rows }
func (f *fakeEmu) CellAt(x, y int) *uv.Cell    { return nil }
func (f *fakeEmu) CursorPosition() uv.Position { return uv.Pos(0, 0) }
func (f *fakeEmu) IsAltScreen() bool           { return false }
func (f *fakeEmu) ScrollbackLen() int          { return len(f.scrollback) }
func (f *fakeEmu) ScrollbackLine(y int) uv.Line {
	if y < 0 || y >= len(f.scrollback) {
		return nil
	}
	return f.scrollback[y]
}
func (f *fakeEmu) Close()                         { f.closed++ }
func (f *fakeEmu) SetResponseSink(func(p []byte)) {}

func TestNewReturnsPairedModelAndSerializer(t *testing.T) {
	m, s := New(80, 24, 500)
	if m == nil || s == nil {
		t.Fatal("New returned nil half")
	}
	if m.Cols() != 80 || m.Rows() != 24 {
		t.Fatalf("size = %dx%d, want 80x24", m.Cols(), m.Rows())
	}
	cols, rows, alt, sb := m.HeaderState()
	if cols != 80 || rows != 24 || alt || sb != 500 {
		t.Fatalf("HeaderState = %d,%d,%v,%d", cols, rows, alt, sb)
	}
}

func TestWriteRecoversParsePanicAndRecreates(t *testing.T) {
	orig := newEmulator
	t.Cleanup(func() { newEmulator = orig })
	newEmulator = func(cols, rows int, cb vt.Callbacks, sb int) emulator {
		return &fakeEmu{cols: cols, rows: rows, panicByte: 0xff}
	}

	m := newVTModel(80, 24, 100)
	m.Write([]byte("clean"))
	if m.Degraded() {
		t.Fatal("model degraded before any panic")
	}
	if m.ParsePanics() != 0 {
		t.Fatalf("ParsePanics = %d before panic", m.ParsePanics())
	}

	m.Write([]byte{0xff})
	if !m.Degraded() {
		t.Fatal("model not degraded after recovered panic")
	}
	if m.ParsePanics() != 1 {
		t.Fatalf("ParsePanics = %d, want 1", m.ParsePanics())
	}
	if m.Cols() != 80 || m.Rows() != 24 {
		t.Fatalf("size after recreate = %dx%d, want 80x24", m.Cols(), m.Rows())
	}
	if len(m.PendingInput()) != 0 {
		t.Fatal("pending input not cleared after recreate")
	}
}

// TestWriteRecoverDiscardsScrollbackContent pins the ACCEPTED RESIDUAL recorded in
// UPSTREAM.md (alongside D1): at this pin scrollback is sourced entirely from the x/vt
// emulator's own buffer, so the parse-panic recreate — which builds a brand-new blank
// emulator via the newEmulator seam — discards the scrollback CONTENT (history) along with
// the visible grid. Only the configured scrollback DEPTH and the title survive. This is a
// self-healing degraded-path corner (the app repaints on its next frame; live clients
// already received the raw bytes), but it is real history loss, so it is pinned here rather
// than left as an undocumented surprise. If a future pin preserves history across recreate,
// this test flips and the doc/UPSTREAM residual is removed.
func TestWriteRecoverDiscardsScrollbackContent(t *testing.T) {
	orig := newEmulator
	t.Cleanup(func() { newEmulator = orig })
	newEmulator = func(cols, rows int, cb vt.Callbacks, sb int) emulator {
		return &fakeEmu{cols: cols, rows: rows, panicByte: 0xff}
	}

	m := newVTModel(80, 24, 100)
	// Simulate accumulated history on the live emulator's own scrollback buffer.
	live := m.emu.(*fakeEmu)
	live.scrollback = []uv.Line{{{Content: "h"}}, {{Content: "i"}}}
	if m.emu.ScrollbackLen() != 2 {
		t.Fatalf("setup: scrollback len = %d, want 2", m.emu.ScrollbackLen())
	}

	m.Write([]byte{0xff}) // parse panic -> recover -> recreate to a fresh blank emulator

	if !m.Degraded() {
		t.Fatal("model not degraded after recovered panic")
	}
	if got := m.emu.ScrollbackLen(); got != 0 {
		t.Fatalf("scrollback content survived recreate (len=%d); the doc/UPSTREAM "+
			"accepted-residual claim that history is lost on recreate is now false — "+
			"update both if history is intentionally preserved", got)
	}
}

func TestCallbacksFeedShadowState(t *testing.T) {
	m := newVTModel(80, 24, 100)

	m.Write([]byte("\x1b]0;both\x07"))
	if m.shadow.title != "both" || m.shadow.iconName != "both" {
		t.Fatalf("OSC 0 title/icon = %q/%q", m.shadow.title, m.shadow.iconName)
	}
	m.Write([]byte("\x1b]2;mytitle\x07"))
	if m.Title() != "mytitle" {
		t.Fatalf("title = %q", m.Title())
	}
	m.Write([]byte("\x1b]1;myicon\x07"))
	if m.shadow.iconName != "myicon" {
		t.Fatalf("icon = %q", m.shadow.iconName)
	}

	m.Write([]byte("\x1b[?1049h"))
	if !m.shadow.altScreen {
		t.Fatal("alt screen flag not set")
	}
	m.Write([]byte("\x1b[?25l"))
	if m.shadow.cursorVisible {
		t.Fatal("cursor visibility not cleared")
	}
	m.Write([]byte("\x1b[4 q"))
	if !m.shadow.cursorShapeSet || m.shadow.cursorShape != vt.CursorUnderline {
		t.Fatalf("cursor style = %v set=%v", m.shadow.cursorShape, m.shadow.cursorShapeSet)
	}

	m.Write([]byte("\x1b[?1000h"))
	if !m.shadow.modes[1000] {
		t.Fatal("mode 1000 not enabled")
	}
	m.Write([]byte("\x1b[?1000l"))
	if m.shadow.modes[1000] {
		t.Fatal("mode 1000 not disabled")
	}

	m.Write([]byte("\x1b]10;rgb:ff/00/00\x07"))
	m.Write([]byte("\x1b]11;rgb:00/ff/00\x07"))
	m.Write([]byte("\x1b]12;rgb:00/00/ff\x07"))
	if !m.shadow.fgSet || !m.shadow.bgSet || !m.shadow.cursorColorSet {
		t.Fatalf("color set flags = %v %v %v", m.shadow.fgSet, m.shadow.bgSet, m.shadow.cursorColorSet)
	}
}

func TestObserveModeIgnoresANSIModes(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Write([]byte("\x1b[4h")) // ANSI insert mode (IRM), not a DEC private mode
	if len(m.shadow.modes) != 0 {
		t.Fatalf("ANSI mode leaked into shadow modes: %v", m.shadow.modes)
	}
}

func TestObserveDefaultColorNilClearsFlag(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.observeDefaultColor(0, color.RGBA{R: 1})
	m.observeDefaultColor(1, color.RGBA{G: 1})
	m.observeDefaultColor(2, color.RGBA{B: 1})
	if !m.shadow.fgSet || !m.shadow.bgSet || !m.shadow.cursorColorSet {
		t.Fatal("color set flags not set")
	}
	m.observeDefaultColor(0, nil)
	m.observeDefaultColor(1, nil)
	m.observeDefaultColor(2, nil)
	if m.shadow.fgSet || m.shadow.bgSet || m.shadow.cursorColorSet {
		t.Fatal("color set flags not cleared by nil")
	}
}

func TestResizeAndForegroundResetAndClose(t *testing.T) {
	m := newVTModel(80, 24, 100)
	m.Resize(100, 40)
	if m.Cols() != 100 || m.Rows() != 40 {
		t.Fatalf("resize size = %dx%d", m.Cols(), m.Rows())
	}

	m.shadow.altScreen = true
	m.shadow.modes[1000] = true
	m.shadow.modes[7] = false
	m.shadow.cursorShapeSet = true
	m.shadow.scrollRegionSet = true
	m.OnForegroundReset()
	if m.shadow.altScreen || m.shadow.modes[1000] || m.shadow.cursorShapeSet || m.shadow.scrollRegionSet {
		t.Fatal("OnForegroundReset did not clear transient state")
	}
	if _, ok := m.shadow.modes[7]; ok {
		t.Fatal("OnForegroundReset did not restore autowrap default")
	}

	m.shadow.title = "x"
	m.Close()
	if m.shadow.title != "" || m.shadow.g0 != 'B' {
		t.Fatal("Close did not reset shadow state")
	}
}

// gridSnapshot reads every visible cell content into a flat string so two screen states
// can be compared exactly. It deliberately reads the emulator's own grid (not the shadow)
// to prove OnForegroundReset leaves the visible buffer physically untouched.
func gridSnapshot(m *vtModel) string {
	var b strings.Builder
	cols, rows := m.emu.Width(), m.emu.Height()
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := m.emu.CellAt(x, y)
			if c == nil || c.Content == "" {
				b.WriteByte(' ')
				continue
			}
			b.WriteString(c.Content)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// TestOnForegroundReset_PreservesScreen is the §13.1-mandated table over all three
// alt-screen entry variants. It pins the coupled §4/§11.1 contract: after a foreground app
// that entered the alt buffer dies, OnForegroundReset must drive x/vt itself out of the alt
// buffer (so emu.IsAltScreen()==false — the serializer's single source of truth), while
// leaving the primary grid, scrollback, title and cursor position legitimately on screen.
func TestOnForegroundReset_PreservesScreen(t *testing.T) {
	// The teardown is the load-bearing payload; assert directly it contains no RIS (ESC c),
	// which would clear the grid+scrollback — the exact opposite of the method's contract.
	if strings.Contains(foregroundResetTeardown, "\x1bc") {
		t.Fatal("teardown contains RIS (ESC c); it must preserve grid + scrollback")
	}

	cases := []struct {
		name  string
		enter string
	}{
		{"DECSET1049", "\x1b[?1049h"},
		{"DECSET47", "\x1b[?47h"},
		{"DECSET1047", "\x1b[?1047h"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVTModel(20, 5, 100)

			// Paint a distinctive primary screen, set a title, and park the cursor at a
			// known interior position — all of which must survive the reset.
			m.Write([]byte("\x1b]2;myshell\x07"))
			m.Write([]byte("alpha\r\nbravo\r\ncharlie"))
			m.Write([]byte("\x1b[2;4H")) // CUP -> row 2, col 4

			wantGrid := gridSnapshot(m)
			wantCursor := m.emu.CursorPosition()
			wantTitle := m.Title()
			wantScrollback := m.emu.ScrollbackLen()

			// A foreground app takes over the alt buffer, then dies WITHOUT resetting it
			// (no ?1049l/?47l/?1047l ever emitted).
			m.Write([]byte(tc.enter))

			m.OnForegroundReset()

			if m.emu.IsAltScreen() {
				t.Fatal("emu still in alt buffer after OnForegroundReset; serializer would " +
					"re-emit ?1049h + the stale alt grid into the idle prompt")
			}
			if m.shadow.altScreen {
				t.Fatal("shadow.altScreen still set after OnForegroundReset")
			}
			if got := gridSnapshot(m); got != wantGrid {
				t.Fatalf("primary grid mutated by reset:\n got %q\nwant %q", got, wantGrid)
			}
			if got := m.emu.CursorPosition(); got != wantCursor {
				t.Fatalf("cursor moved by reset: got %v want %v", got, wantCursor)
			}
			if got := m.Title(); got != wantTitle {
				t.Fatalf("title changed by reset: got %q want %q", got, wantTitle)
			}
			if got := m.emu.ScrollbackLen(); got != wantScrollback {
				t.Fatalf("scrollback length changed by reset: got %d want %d", got, wantScrollback)
			}

			// Idempotent: a second edge (e.g. another debounced sample) is a no-op.
			m.OnForegroundReset()
			if m.emu.IsAltScreen() || m.shadow.altScreen {
				t.Fatal("second OnForegroundReset re-entered alt buffer")
			}
			if got := gridSnapshot(m); got != wantGrid {
				t.Fatalf("second reset mutated grid:\n got %q\nwant %q", got, wantGrid)
			}
			if got := m.emu.CursorPosition(); got != wantCursor {
				t.Fatalf("second reset moved cursor: got %v want %v", got, wantCursor)
			}
		})
	}
}

// TestOnForegroundReset_DrivesEmuOutOfAltScreen reproduces the reviewer's empirical repro:
// entering ?1049h, calling OnForegroundReset, then Serialize must NOT yield an emulator that
// reports alt-screen nor a payload that re-asserts ?1049h into the idle prompt.
func TestOnForegroundReset_DrivesEmuOutOfAltScreen(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.Write([]byte("idle$ "))
	m.Write([]byte("\x1b[?1049h"))
	m.Write([]byte("ALT-APP-CONTENT"))
	if !m.emu.IsAltScreen() {
		t.Fatal("setup: ?1049h did not put emu in alt buffer")
	}

	m.OnForegroundReset()

	if m.emu.IsAltScreen() {
		t.Fatal("emu still in alt buffer after OnForegroundReset")
	}
	payload := string(vtSerializer{}.Serialize(m))
	if strings.Contains(payload, ansi.SetMode(ansi.DECMode(1049))) {
		t.Fatalf("serialized payload re-asserts ?1049h into idle prompt:\n%q", payload)
	}
}

func TestTrackPendingPartial(t *testing.T) {
	m := newVTModel(80, 24, 100)

	m.Write([]byte("plain text"))
	if m.PendingInput() != nil {
		t.Fatalf("ground-state input left pending: %q", m.PendingInput())
	}

	m.Write([]byte("done\x1b[31")) // trailing incomplete CSI
	if got := string(m.PendingInput()); got != "\x1b[31" {
		t.Fatalf("pending = %q, want ESC[31", got)
	}

	m.Write([]byte("m")) // completes the carried CSI
	if m.PendingInput() != nil {
		t.Fatalf("pending not cleared after completion: %q", m.PendingInput())
	}

	long := append([]byte("\x1b]0;"), bytes.Repeat([]byte("a"), maxPendingPartial+10)...)
	m.Write(long)
	if m.PendingInput() != nil {
		t.Fatalf("oversized partial not dropped: %d bytes", len(m.PendingInput()))
	}
}

// TestPendingInputNeverPrintableLeadingBlob is the re-attach corruption regression. A
// realistic multi-write Claude-style frame — box drawing, "What's new" tips, footer, input
// line, cursor-park — is fed across several chunks. Its final chunk ends AFTER complete draw
// commands (ground state), so PendingInput() MUST be nil. Before the UTF-8-transparency fix
// the box-drawing glyph "▘" (U+2598 = E2 96 98) flipped scanPartial into an 8-bit-C1 string
// state on byte 0x98, so PendingInput() returned a hundreds-of-bytes PRINTABLE-leading blob
// that Attach appended after the clean serialize — painting app chrome into the input line.
func TestPendingInputNeverPrintableLeadingBlob(t *testing.T) {
	m := newVTModel(170, 59, 1000)

	// Multi-write frame. Box glyphs (U+2500 family, U+2598/U+259D quadrants) carry bytes in
	// the 0x80-0x9F C1 range as UTF-8 continuation/lead bytes; interleaved with real CSI
	// draw commands, exactly like Claude Code's welcome frame.
	chunks := []string{
		"\x1b[2J\x1b[H",
		"╭" + strings.Repeat("─", 60) + "╮\r\n",
		"│ ▘ ▝▝  \x1b[54G\x1b[2m│\x1b[56G\x1b[39m\x1b[22mAdded\x1b[62Gsupport\x1b[70Gfor\r\n",
		"│ \x1b[38;2;153;153;153mOpus with me… · Claude Max\x1b[39m\r\n",
		"╰" + strings.Repeat("─", 60) + "╯\r\n",
		"\x1b[59;3H> ", // park cursor on the input line, ground state
	}
	for _, c := range chunks {
		m.Write([]byte(c))
	}

	if pi := m.PendingInput(); pi != nil {
		t.Fatalf("ground-terminated frame left pending input: len=%d printableLeading=%v %q",
			len(pi), len(pi) > 0 && pi[0] != 0x1b, pi)
	}

	// Assert the concatenation Attach performs (clean serialize + PendingInput) never begins
	// its appended tail with a printable byte, i.e. there is no printable-leading multi-
	// command run leaking past the serialize.
	redraw := vtSerializer{}.Serialize(m)
	tail := m.PendingInput()
	full := append(append([]byte(nil), redraw...), tail...)
	if len(tail) > 0 && tail[0] != 0x1b {
		t.Fatalf("appended pending tail is printable-leading: %q", tail)
	}
	if bytes.Contains(full, []byte("Added\x1b[62Gsupport")) && len(tail) > 0 {
		t.Fatalf("frame content leaked into appended pending tail: %q", tail)
	}

	// Genuine mid-sequence attach boundary: the final chunk ends INSIDE a CSI. PendingInput
	// must now be a SHORT ESC-leading tail (the incomplete sequence only), never the frame.
	m2 := newVTModel(170, 59, 1000)
	for _, c := range chunks[:len(chunks)-1] {
		m2.Write([]byte(c))
	}
	m2.Write([]byte("▘▝\x1b[38;2;153")) // glyphs then an incomplete SGR
	pi := m2.PendingInput()
	if len(pi) == 0 || pi[0] != 0x1b {
		t.Fatalf("genuine partial not returned as ESC-leading tail: %q", pi)
	}
	if string(pi) != "\x1b[38;2;153" {
		t.Fatalf("partial tail = %q, want the incomplete CSI only", pi)
	}
	if len(pi) > 16 {
		t.Fatalf("partial tail not bounded-small: len=%d", len(pi))
	}
}

// TestResponseSink_ReceivesCPRAnswer proves the model's SetResponseSink taps the
// emulator's device-query replies (spec §3.8): a cursor-position-report query
// (ESC[6n) written into the model must reach the installed sink as a CPR reply.
func TestResponseSink_ReceivesCPRAnswer(t *testing.T) {
	m, _ := New(20, 5, 100)
	t.Cleanup(func() { m.Close() })
	got := make(chan []byte, 4)
	m.SetResponseSink(func(p []byte) {
		cp := append([]byte(nil), p...)
		got <- cp
	})
	m.Write([]byte("\x1b[6n")) // cursor position report query
	select {
	case reply := <-got:
		assert.Regexp(t, `\x1b\[\d+;\d+R`, string(reply), "CPR reply expected")
	case <-time.After(2 * time.Second):
		t.Fatal("no device-query reply reached the sink")
	}
}

func TestModeReassertedThroughSerializerRoundTrip(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.Write([]byte("\x1b[?2004h"))
	payload := vtSerializer{}.Serialize(m)
	if !strings.Contains(string(payload), ansi.SetMode(ansi.DECMode(2004))) {
		t.Fatal("bracketed-paste mode not re-asserted in payload")
	}
}
