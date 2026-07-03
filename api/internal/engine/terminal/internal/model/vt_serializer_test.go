package model

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
)

func serialize(
	m *vtModel,
) string {
	return string(vtSerializer{}.Serialize(m))
}

func TestSerializePreambleAlwaysSoftResetsAndForcesAutowrap(t *testing.T) {
	m := newVTModel(20, 5, 100)
	out := serialize(m)
	want := decstr + ansi.SetMode(ansi.DECMode(7))
	if !strings.HasPrefix(out, want) {
		t.Fatalf("payload does not start with DECSTR + ?7h: %q", out[:min(len(out), 16)])
	}
	if !strings.HasSuffix(out, ansi.ResetStyle) {
		t.Fatal("payload does not end in ground-state SGR reset")
	}
}

func TestSerializeDefaultColors(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.shadow.setDefaultColor(0, color.RGBA{R: 0xff, A: 0xff})
	m.shadow.setDefaultColor(1, color.RGBA{G: 0xff, A: 0xff})
	m.shadow.setDefaultColor(2, color.RGBA{B: 0xff, A: 0xff})
	out := serialize(m)
	for _, want := range []string{
		"\x1b]10;rgb:ffff/0000/0000\x1b\\",
		"\x1b]11;rgb:0000/ffff/0000\x1b\\",
		"\x1b]12;rgb:0000/0000/ffff\x1b\\",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("payload missing color OSC %q", want)
		}
	}
}

func TestSerializeAltScreenEntersAndGates(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.Write([]byte("primary line\r\n"))
	m.Write([]byte("\x1b[?1049h"))
	out := serialize(m)
	if !strings.Contains(out, ansi.SetMode(ansi.DECMode(1049))) {
		t.Fatal("alt-screen enter not emitted")
	}
}

func TestSerializeScrollbackEmittedWhenPrimary(t *testing.T) {
	m := newVTModel(20, 4, 100)
	for i := 0; i < 12; i++ {
		m.Write([]byte(fmt.Sprintf("row%d\r\n", i)))
	}
	if m.emu.ScrollbackLen() == 0 {
		t.Fatal("test setup did not produce scrollback")
	}
	out := serialize(m)
	if !strings.Contains(out, "row0") {
		t.Fatal("oldest scrollback line not serialized")
	}
}

func TestSerializeModesAndAutowrapReset(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.shadow.modes[1000] = true
	m.shadow.modes[1006] = true
	m.shadow.modes[7] = false // app disabled autowrap
	out := serialize(m)
	if !strings.Contains(out, ansi.SetMode(ansi.DECMode(1000))) {
		t.Fatal("mode 1000 not emitted")
	}
	if !strings.Contains(out, ansi.SetMode(ansi.DECMode(1006))) {
		t.Fatal("mode 1006 not emitted")
	}
	if !strings.Contains(out, ansi.ResetMode(ansi.DECMode(7))) {
		t.Fatal("autowrap reset not emitted")
	}
}

func TestSerializeAutowrapEnabledOmitsReset(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.shadow.modes[7] = true
	out := serialize(m)
	if strings.Contains(out, ansi.ResetMode(ansi.DECMode(7))) {
		t.Fatal("autowrap reset emitted despite autowrap on")
	}
}

func TestSerializeScrollRegion(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.shadow.scrollRegionSet = true
	m.shadow.scrollTop, m.shadow.scrollBottom = 2, 4
	out := serialize(m)
	if !strings.Contains(out, ansi.SetTopBottomMargins(2, 4)) {
		t.Fatal("DECSTBM not emitted for partial region")
	}
}

func TestSerializeFullScreenRegionSkipped(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.shadow.scrollRegionSet = true
	m.shadow.scrollTop, m.shadow.scrollBottom = 1, 6
	out := serialize(m)
	if strings.Contains(out, ansi.SetTopBottomMargins(1, 6)) {
		t.Fatal("DECSTBM emitted for full-screen region")
	}
}

func TestSerializeCursorAbsolute(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.Write([]byte("\x1b[3;5H")) // row3 col5
	out := serialize(m)
	if !strings.Contains(out, ansi.CursorPosition(5, 3)) {
		t.Fatalf("absolute CUP not emitted: %q", out)
	}
}

func TestSerializeCursorOriginRelative(t *testing.T) {
	m := newVTModel(20, 6, 100)
	m.Write([]byte("\x1b[4;5H")) // absolute row4 col5
	m.shadow.modes[6] = true
	m.shadow.scrollRegionSet = true
	m.shadow.scrollTop, m.shadow.scrollBottom = 2, 6
	out := serialize(m)
	if !strings.Contains(out, ansi.SetMode(ansi.DECMode(6))) {
		t.Fatal("origin mode not emitted")
	}
	if !strings.Contains(out, ansi.CursorPosition(5, 3)) { // 4 - (2-1) = 3
		t.Fatalf("region-relative CUP not emitted: %q", out)
	}
}

// TestSerializeOriginModeByteOrderAndDecode is the §13.1-mandated golden byte-ORDER guard
// for the headline §6 ordering fix. The other serializer tests assert only presence via
// strings.Contains and would still pass if writeCursor regressed to emit the CUP before
// ?6h, or DECSTBM after the cursor — both substrings are present regardless of order. This
// test pins the two order-sensitive properties §6/§13.1 call out:
//
//	(b) step 11 DECSTBM lands AFTER the grid paint (so the region clamp does not corrupt
//	    the full-screen CR/LF flow of the grid rows), and
//	(a) step 12 ?6h lands BEFORE the step 13 CUP, with the CUP in region-relative
//	    coordinates — proven by an origin-mode-active DECODE case: the payload is replayed
//	    into a fresh vt.Emulator and its decoded CursorPosition must equal the model's real
//	    cell. If the order were reversed (CUP before ?6h, which re-homes the cursor to the
//	    region top, or the CUP emitted absolute), the decoded cursor would NOT match.
//
// The whole state is driven through real Writes (x/vt's EnableMode callback sets
// shadow.modes[6]; escan sets the scroll region), so this is an end-to-end golden, not a
// hand-poked shadow.
func TestSerializeOriginModeByteOrderAndDecode(t *testing.T) {
	const cols, rows = 20, 6
	m := newVTModel(cols, rows, 100)
	m.Write([]byte("MARKER"))    // identifiable grid content on row 0 (above the region)
	m.Write([]byte("\x1b[2;6r")) // DECSTBM region rows 2..6
	m.Write([]byte("\x1b[?6h"))  // origin mode ON
	m.Write([]byte("\x1b[3;5H")) // region-relative CUP -> absolute X=4, Y=3

	// Sanity: the model really is in the state the golden depends on.
	if !m.shadow.modes[6] {
		t.Fatal("setup: origin mode not recorded in shadow")
	}
	if pos := m.emu.CursorPosition(); pos.X != 4 || pos.Y != 3 {
		t.Fatalf("setup: model cursor = %v, want {4,3}", pos)
	}

	out := serialize(m)

	// (b)+(a) explicit index order: grid(MARKER) < DECSTBM < ?6h < final region-relative CUP.
	marker := strings.Index(out, "MARKER")
	decstbm := strings.Index(out, ansi.SetTopBottomMargins(2, 6)) // ESC[2;6r
	originSet := strings.Index(out, ansi.SetMode(ansi.DECMode(6)))
	cup := strings.Index(out, "\x1b[3;5H") // region-relative CUP, unique in the payload
	for name, idx := range map[string]int{
		"MARKER": marker, "DECSTBM": decstbm, "?6h": originSet, "CUP": cup,
	} {
		if idx < 0 {
			t.Fatalf("payload missing %s: %q", name, out)
		}
	}
	if !(marker < decstbm && decstbm < originSet && originSet < cup) {
		t.Fatalf("byte order violated: MARKER@%d DECSTBM@%d ?6h@%d CUP@%d\npayload=%q",
			marker, decstbm, originSet, cup, out)
	}

	// (a) decode: replay the payload into a fresh emulator; its cursor MUST land on the
	// model's real cell. This is the assertion that fails if ?6h/CUP were reordered.
	fresh := vt.NewEmulator(cols, rows)
	if _, err := fresh.Write([]byte(out)); err != nil {
		t.Fatalf("fresh emulator write: %v", err)
	}
	if want, got := m.emu.CursorPosition(), fresh.CursorPosition(); want != got {
		t.Fatalf("origin-mode cursor decoded to %v, want model %v (ordering regression?)", got, want)
	}
}

func TestSerializeCursorShapeAndVisibility(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.shadow.cursorShapeSet = true
	m.shadow.cursorShape = vt.CursorBar
	m.shadow.cursorBlink = true
	m.shadow.cursorVisible = false
	out := serialize(m)
	if !strings.Contains(out, ansi.SetCursorStyle(5)) {
		t.Fatal("DECSCUSR bar-blink not emitted")
	}
	if !strings.Contains(out, ansi.ResetMode(ansi.DECMode(25))) {
		t.Fatal("cursor-hidden DECTCEM not emitted")
	}
}

func TestSerializeCursorVisibleEmitted(t *testing.T) {
	m := newVTModel(20, 5, 100)
	out := serialize(m)
	if !strings.Contains(out, ansi.SetMode(ansi.DECMode(25))) {
		t.Fatal("cursor-visible DECTCEM not emitted")
	}
}

func TestSerializeTitleAndIconSanitized(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.shadow.iconName = "ic\x07on"
	m.shadow.title = "ti\x1b[31mtle"
	out := serialize(m)
	if !strings.Contains(out, "\x1b]1;icon\x1b\\") {
		t.Fatalf("sanitized icon OSC missing: %q", out)
	}
	if !strings.Contains(out, "\x1b]2;ti[31mtle\x1b\\") {
		t.Fatalf("sanitized title OSC missing: %q", out)
	}
}

func TestSerializeOmitsEmptyTitle(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.shadow.title = "\x1b\x07" // sanitizes to empty
	out := serialize(m)
	if strings.Contains(out, "\x1b]2;") {
		t.Fatal("empty title still emitted an OSC 2")
	}
}

func TestSerializeCharset(t *testing.T) {
	cases := []struct {
		name      string
		g0, g1    byte
		glLock    int
		wantDesig []string
		wantShift string
		noShift   bool
	}{
		{"g0 special no shift", '0', 'B', 0, []string{"\x1b(0"}, "", true},
		{"g1 special locks SI", 'B', '0', 0, []string{"\x1b)0"}, "\x0f", false},
		{"locking shift SO", 'B', '0', 1, []string{"\x1b)0"}, "\x0e", false},
		{"default none", 'B', 'B', 0, nil, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newVTModel(20, 5, 100)
			m.shadow.g0, m.shadow.g1, m.shadow.glLock = tc.g0, tc.g1, tc.glLock
			out := serialize(m)
			for _, d := range tc.wantDesig {
				if !strings.Contains(out, d) {
					t.Fatalf("designation %q missing", d)
				}
			}
			switch {
			case tc.noShift:
				if strings.Contains(out, "\x0e") || strings.Contains(out, "\x0f") {
					t.Fatal("unexpected locking shift emitted")
				}
			default:
				if !strings.Contains(out, tc.wantShift) {
					t.Fatalf("locking shift %q missing", tc.wantShift)
				}
			}
		})
	}
}

func TestSerializeWrongModelTypePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Serialize did not panic on mismatched model type")
		}
	}()
	vtSerializer{}.Serialize(otherModel{})
}

type otherModel struct{}

func (otherModel) Write([]byte)                       {}
func (otherModel) Resize(int, int)                    {}
func (otherModel) OnForegroundReset()                 {}
func (otherModel) PendingInput() []byte               { return nil }
func (otherModel) Title() string                      { return "" }
func (otherModel) Cols() int                          { return 0 }
func (otherModel) Rows() int                          { return 0 }
func (otherModel) HeaderState() (int, int, bool, int) { return 0, 0, false, 0 }
func (otherModel) ModelBytes() int64                  { return 0 }
func (otherModel) Close()                             {}
func (otherModel) SetResponseSink(func(p []byte))     {}

func TestEncodeGridRowOutOfBounds(t *testing.T) {
	m := newVTModel(5, 3, 100)
	if got := encodeGridRow(m.emu, 5, 999); got != "" {
		t.Fatalf("out-of-bounds row encoded to %q, want empty", got)
	}
}

func TestEncodeLine(t *testing.T) {
	red := uv.Style{Fg: color.RGBA{R: 0xff, A: 0xff}}

	t.Run("trim trailing blanks", func(t *testing.T) {
		cells := []uv.Cell{{Content: "a", Width: 1}, uv.EmptyCell, uv.EmptyCell}
		if got := encodeLine(cells, 3, true); got != "a" {
			t.Fatalf("got %q, want %q", got, "a")
		}
	})

	t.Run("no trim keeps blanks", func(t *testing.T) {
		cells := []uv.Cell{{Content: "a", Width: 1}, uv.EmptyCell, uv.EmptyCell}
		if got := encodeLine(cells, 3, false); got != "a  " {
			t.Fatalf("got %q, want %q", got, "a  ")
		}
	})

	t.Run("no trim empty-content cell becomes space", func(t *testing.T) {
		cells := []uv.Cell{{Content: "a", Width: 1}, {Content: "", Width: 1}}
		if got := encodeLine(cells, 2, false); got != "a " {
			t.Fatalf("got %q, want %q", got, "a ")
		}
	})

	t.Run("wide cell skips continuation", func(t *testing.T) {
		cells := []uv.Cell{{Content: "世", Width: 2}, {Content: "", Width: 0}, {Content: "x", Width: 1}}
		if got := encodeLine(cells, 3, true); got != "世x" {
			t.Fatalf("got %q, want %q", got, "世x")
		}
	})

	t.Run("styled cell resets pen", func(t *testing.T) {
		cells := []uv.Cell{{Content: "Z", Width: 1, Style: red}}
		got := encodeLine(cells, 1, true)
		if !strings.HasSuffix(got, ansi.ResetStyle) {
			t.Fatalf("styled row %q did not reset pen", got)
		}
		if !strings.Contains(got, "Z") {
			t.Fatalf("styled row %q lost content", got)
		}
	})

	t.Run("width exceeds cells", func(t *testing.T) {
		cells := []uv.Cell{{Content: "a", Width: 1}}
		if got := encodeLine(cells, 10, true); got != "a" {
			t.Fatalf("got %q, want %q", got, "a")
		}
	})
}

func TestCellBlank(t *testing.T) {
	red := uv.Style{Fg: color.RGBA{R: 0xff, A: 0xff}}
	cases := []struct {
		name string
		cell uv.Cell
		want bool
	}{
		{"space", uv.Cell{Content: " ", Width: 1}, true},
		{"empty content", uv.Cell{Content: "", Width: 0}, true},
		{"glyph", uv.Cell{Content: "x", Width: 1}, false},
		{"styled space", uv.Cell{Content: " ", Width: 1, Style: red}, false},
		{"linked space", uv.Cell{Content: " ", Width: 1, Link: uv.Link{URL: "u"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.cell
			if got := cellBlank(&c); got != tc.want {
				t.Fatalf("cellBlank(%+v) = %v, want %v", tc.cell, got, tc.want)
			}
		})
	}
}

func TestFormatColor(t *testing.T) {
	got := formatColor(color.RGBA{R: 0xff, G: 0x00, B: 0x80, A: 0xff})
	if got != "rgb:ffff/0000/8080" {
		t.Fatalf("formatColor = %q", got)
	}
}

func TestDecscusr(t *testing.T) {
	cases := []struct {
		shape vt.CursorStyle
		blink bool
		want  int
	}{
		{vt.CursorBlock, true, 1},
		{vt.CursorBlock, false, 2},
		{vt.CursorUnderline, true, 3},
		{vt.CursorUnderline, false, 4},
		{vt.CursorBar, true, 5},
		{vt.CursorBar, false, 6},
	}
	for _, tc := range cases {
		if got := decscusr(tc.shape, tc.blink); got != tc.want {
			t.Fatalf("decscusr(%v,%v) = %d, want %d", tc.shape, tc.blink, got, tc.want)
		}
	}
}

func TestSanitizeOSCText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"drop esc", "ab\x1bc", "abc"},
		{"drop bel", "a\x07b", "ab"},
		{"drop del", "a\x7fb", "ab"},
		{"drop c1", "ab", "ab"},
		{"keep utf8", "héllo", "héllo"},
		{"invalid utf8 byte dropped", string([]byte{0xff, 'a'}), "a"},
		{"truncated multibyte lead dropped", string([]byte{0xe2, 'a'}), "a"},
		{"genuine replacement char kept", "�a", "�a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeOSCText(tc.in); got != tc.want {
				t.Fatalf("sanitizeOSCText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("caps at max runes", func(t *testing.T) {
		got := sanitizeOSCText(strings.Repeat("x", maxOSCTextRunes+50))
		if len([]rune(got)) != maxOSCTextRunes {
			t.Fatalf("len = %d, want %d", len([]rune(got)), maxOSCTextRunes)
		}
	})
}

// TestRoundTripThroughXVT is a SELF round-trip, NOT the mandated conformance oracle.
//
// SCOPE / DEFERRED GATE: the spec (§6.1, §13.2, §15 P0 exit-gate) designates xterm.js's
// SerializeAddon as THE conformance oracle and requires empirical buffer-equivalence of our
// serialized output against a fresh xterm.js across real-app fixtures. This test instead
// feeds the payload back into another x/vt emulator (x/vt -> serialize -> x/vt). That proves
// the serializer is a correct INVERSE of x/vt's own parser — a useful fixpoint that catches
// encoder regressions — but it is structurally incapable of catching x/vt-vs-xterm.js
// divergences in exactly the behaviors the spec says to verify against the oracle: DECSTR's
// implementation-defined autowrap effect, region-relative CUP under origin mode, and
// soft-wrap CR/LF handling. The real client is xterm.js, and its buffer-equivalence is
// UNPROVEN by this foundation.
//
// The xterm.js SerializeAddon buffer-equivalence harness (committed @xterm/xterm +
// @xterm/addon-serialize lockfile, real-app PTY fixtures) is an explicitly DEFERRED §15 P0
// exit-gate item, tracked as a follow-up; it is NOT delivered in this foundational-packages
// workflow. Do not mistake this self-round-trip for the mandated oracle proof.
func TestRoundTripThroughXVT(t *testing.T) {
	const cols, rows = 40, 10
	m, s := New(cols, rows, 1000)

	var in []byte
	for i := 0; i < 18; i++ {
		in = append(in, []byte(fmt.Sprintf("history line %d\r\n", i))...)
	}
	in = append(in, []byte("\x1b[31mRED\x1b[m and plain text")...)
	in = append(in, []byte("\x1b[?1000h\x1b[?2004h")...)
	in = append(in, []byte("\x1b[4;3H")...) // park cursor away from margins
	m.Write(in)

	payload := s.Serialize(m)

	modes := map[int]bool{}
	fresh := vt.NewEmulator(cols, rows)
	fresh.SetScrollbackSize(1000)
	fresh.SetCallbacks(vt.Callbacks{
		EnableMode: func(md ansi.Mode) {
			if dm, ok := md.(ansi.DECMode); ok {
				modes[dm.Mode()] = true
			}
		},
		DisableMode: func(md ansi.Mode) {
			if dm, ok := md.(ansi.DECMode); ok {
				modes[dm.Mode()] = false
			}
		},
	})
	if _, err := fresh.Write(payload); err != nil {
		t.Fatalf("fresh emulator write: %v", err)
	}

	vm := m.(*vtModel)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			want := vm.emu.CellAt(x, y)
			got := fresh.CellAt(x, y)
			assertCellEqual(t, x, y, want, got)
		}
	}

	// Scrollback is the headline §6/§13.3 invariant: the serialized scrollback flow must
	// replay into the fresh emulator's history oldest-first, newest immediately above the
	// grid top, with no line dropped and no grid row spilled. The visible-grid assertion
	// above is blind to all of that (it stays correct even if scrollback were reversed,
	// truncated, or off by one), so pin scrollback boundary + order + depth directly.
	sbLen := vm.emu.ScrollbackLen()
	if sbLen == 0 {
		t.Fatal("setup produced no scrollback; round-trip cannot guard the scrollback flow")
	}
	// Depth + boundary: the fresh emulator parsed the whole payload and pushed exactly the
	// pre-grid physical lines into history. Equal length proves no line was dropped and no
	// grid row leaked into (or out of) scrollback — i.e. the total reconstructed physical
	// lines == ScrollbackLen()+rows, with the boundary in the right place.
	if got := fresh.ScrollbackLen(); got != sbLen {
		t.Fatalf("scrollback depth mismatch: model %d, fresh %d (boundary/drop regression; "+
			"total physical lines must be ScrollbackLen()+rows=%d)", sbLen, got, sbLen+rows)
	}
	// Order + content: line 0 is oldest, line sbLen-1 is newest (immediately above the grid
	// top). Compare cell-for-cell, in order, so a chronological reversal or a swapped line
	// fails here even though the visible grid is untouched.
	for y := 0; y < sbLen; y++ {
		want := vm.emu.ScrollbackLine(y)
		got := fresh.Scrollback().Line(y)
		for x := 0; x < cols; x++ {
			assertCellEqual(t, x, y, lineCellAt(want, x), lineCellAt(got, x))
		}
	}
	// Anchor the oldest/newest sentinels so the order assertion above is self-evidently
	// chronological and not accidentally symmetric.
	if c := cellContent(lineCellAt(vm.emu.ScrollbackLine(0), 0)); c != "h" {
		t.Fatalf("oldest scrollback line does not start with %q (history line 0): got %q", "h", c)
	}

	if want, got := vm.emu.CursorPosition(), fresh.CursorPosition(); want != got {
		t.Fatalf("cursor mismatch: model %v, fresh %v", want, got)
	}
	if !modes[1000] {
		t.Error("mouse mode ?1000 not restored in fresh emulator")
	}
	if !modes[2004] {
		t.Error("bracketed-paste ?2004 not restored in fresh emulator")
	}
}

// lineCellAt returns a pointer to cell x of line l, or nil when x is past the line's stored
// width. encodeLine trims trailing blanks, so the model's full-width scrollback line and the
// fresh emulator's reconstructed line can carry different trailing-cell counts; treating an
// out-of-range index as a blank (nil) cell lets assertCellEqual compare them by visible
// content without spuriously failing on equivalent trailing whitespace.
func lineCellAt(
	l uv.Line,
	x int,
) *uv.Cell {
	if x < 0 || x >= len(l) {
		return nil
	}
	return &l[x]
}

// TestSoftWrapResidualHardBreaksButPreservesCells pins the §6.2-family accepted residual
// documented in the package doc: the pinned x/vt commit surfaces no per-row soft-wrap bit,
// so the serializer emits a soft-wrapped logical line HARD-broken (full row + CR/LF + the
// continuation) instead of as one continuous autowrap flow. The test proves both halves of
// the accepted contract: (1) the wrap semantic is lost — a CR/LF separator IS emitted
// between the two physical rows of one soft-wrapped logical line; (2) the visible CELLS are
// nonetheless preserved exactly through a round-trip into a fresh emulator. If a future
// x/vt commit surfaces the wrap bit and steps 5/7 implement wrap-awareness, the CR/LF
// assertion flips and this test is rewritten alongside the package-doc residual.
func TestSoftWrapResidualHardBreaksButPreservesCells(t *testing.T) {
	const cols, rows = 10, 4
	m, s := New(cols, rows, 100)
	// 20 printable chars at width 10 fill row0 (0-9) and soft-wrap into row1 (A-J): one
	// logical line spanning two physical rows with no intervening CR/LF in the input.
	m.Write([]byte("0123456789ABCDEFGHIJ"))

	payload := string(s.Serialize(m))

	// (1) The wrap is lost: the two physical rows are separated by a hard CR/LF. A
	// wrap-aware serializer would emit "0123456789ABCDEFGHIJ" with NO separator.
	if !strings.Contains(payload, "0123456789\r\nABCDEFGHIJ") {
		t.Fatalf("expected hard CR/LF between the soft-wrapped rows; payload=%q", payload)
	}
	if strings.Contains(payload, "0123456789ABCDEFGHIJ") {
		t.Fatal("payload emitted the wrapped line as one continuous flow; the residual no " +
			"longer holds — implement steps 5/7 wrap-awareness and update the package doc")
	}

	// (2) The visible cells survive exactly through the round-trip into a fresh emulator.
	fresh := vt.NewEmulator(cols, rows)
	if _, err := fresh.Write([]byte(payload)); err != nil {
		t.Fatalf("fresh emulator write: %v", err)
	}
	vm := m.(*vtModel)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			assertCellEqual(t, x, y, vm.emu.CellAt(x, y), fresh.CellAt(x, y))
		}
	}
}

func assertCellEqual(
	t *testing.T,
	x int,
	y int,
	want *uv.Cell,
	got *uv.Cell,
) {
	t.Helper()
	wc, gc := cellContent(want), cellContent(got)
	if wc != gc {
		t.Fatalf("cell (%d,%d) content: model %q, fresh %q", x, y, wc, gc)
	}
	if want == nil || got == nil {
		return
	}
	if !want.Style.Equal(&got.Style) {
		t.Fatalf("cell (%d,%d) style: model %+v, fresh %+v", x, y, want.Style, got.Style)
	}
}

func cellContent(
	c *uv.Cell,
) string {
	if c == nil || c.Content == "" {
		return " "
	}
	return c.Content
}
