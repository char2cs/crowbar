package model

import (
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plainText concatenates each cell's content into a single string, mirroring
// cellContent's blank-cell handling in vt_serializer_test.go.
func plainText(cells []uv.Cell) string {
	var b strings.Builder
	for i := range cells {
		c := cells[i].Content
		if c == "" {
			c = " "
		}
		b.WriteString(c)
	}
	return b.String()
}

// scrollbackStrings renders a model's scrollback lines as trimmed plain text.
func scrollbackStrings(m TerminalModel) []string {
	vm := m.(*vtModel)
	n := vm.emu.ScrollbackLen()
	out := make([]string, 0, n)
	for y := 0; y < n; y++ {
		line := vm.emu.ScrollbackLine(y)
		cells := make([]uv.Cell, len(line))
		copy(cells, line)
		out = append(out, strings.TrimRight(plainText(cells), " "))
	}
	return out
}

// newTestModel mirrors vt_model_test.go's construction.
func newTestModel(t *testing.T, cols, rows int) (TerminalModel, Serializer) {
	t.Helper()
	m, s := New(cols, rows, 200)
	t.Cleanup(func() { m.Close() })
	return m, s
}

func TestDiffEmitter_UnprimedNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	data, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe, "an unprimed emitter must demand a keyframe")
	assert.Nil(t, data)
}

func TestDiffEmitter_NoChangeEmitsNothing(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	m.Write([]byte("hello"))
	e := NewDiffEmitter()
	e.Prime(m)
	data, needKeyframe := e.Emit(m)
	assert.False(t, needKeyframe)
	assert.Empty(t, data, "no model change → no bytes")
}

func TestDiffEmitter_DirtyLineRewritten(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	m.Write([]byte("hello"))
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("X")) // row 0 changes
	data, needKeyframe := e.Emit(m)
	require.False(t, needKeyframe)
	s := string(data)
	// Row 0 rewritten in place: absolute cursor position to row 1 col 1 + content.
	assert.Contains(t, s, "\x1b[1;1H", "dirty row must be addressed absolutely")
	assert.Contains(t, s, "helloX")
	// Rows 1-4 untouched: no addressing of row 2..5.
	assert.NotContains(t, s, "\x1b[2;1H")
}

func TestDiffEmitter_ResizeNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Resize(30, 5)
	_, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe, "dimension change must invalidate the diff base")
}

func TestDiffEmitter_InvalidateForcesKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	e.Invalidate()
	_, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe)
}

func TestDiffEmitter_AltScreenFlipNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?1049h")) // enter alt screen
	_, needKeyframe := e.Emit(m)
	assert.True(t, needKeyframe, "alt-screen flip must invalidate the diff base")
}

func TestDiffEmitter_PrimeAfterKeyframeResumesDiffing(t *testing.T) {
	m, _ := newTestModel(t, 20, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Resize(30, 5)
	_, need := e.Emit(m)
	require.True(t, need)
	e.Prime(m) // caller emitted the keyframe; emitter re-bases
	data, need := e.Emit(m)
	assert.False(t, need)
	assert.Empty(t, data)
}

func TestDiffEmitter_ScrollbackDeltaEmittedBeforeScreenDiff(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)

	// Write 5 numbered lines into a 3-row screen: 2+ lines commit to scrollback.
	m.Write([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive"))
	data, need := e.Emit(m)
	require.False(t, need)
	s := string(data)

	// Committed scrollback lines are painted onto the top rows and then
	// scrolled out (write-then-scroll) so the CLIENT gains them in its own
	// history.
	assert.Contains(t, s, "one")
	assert.Contains(t, s, "two")
	// The scrollback flow (which also uses "\x1b[1;1H" to paint its own top
	// row) must precede the final screen-diff repaint. Since both phases can
	// address row 1, detect the screen-diff repaint's CUP as the LAST
	// occurrence of "\x1b[1;1H" and confirm the scrollback content ("one")
	// appears before it.
	sbIdx := strings.Index(s, "one")
	screenIdx := strings.LastIndex(s, "\x1b[1;1H")
	require.GreaterOrEqual(t, screenIdx, 0)
	assert.Less(t, sbIdx, screenIdx, "scrollback delta must precede the final screen-diff repaint")
}

func TestDiffEmitter_NoScrollbackDeltaWhenNoneCommitted(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	m.Write([]byte("hi"))
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("!"))
	data, need := e.Emit(m)
	require.False(t, need)
	// Exactly one dirty row, no scroll flow (no bare "\n" scroll writes).
	assert.Equal(t, 1, strings.Count(string(data), "\x1b[1;1H"))
}

func TestDiffEmitter_ClientScrollbackReceivesCommittedContent(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	sim, _ := newTestModel(t, 10, 3) // client simulator
	e := NewDiffEmitter()
	e.Prime(m)

	m.Write([]byte("one\r\ntwo\r\nthree\r\nfour\r\nfive"))
	data, need := e.Emit(m)
	require.False(t, need)
	sim.Write(data)

	vmAuth := m.(*vtModel)
	simSB := scrollbackStrings(sim)
	require.Equal(t, vmAuth.emu.ScrollbackLen(), len(simSB), "sim must gain exactly the committed line count")
	for y := 0; y < vmAuth.emu.ScrollbackLen(); y++ {
		authLine := vmAuth.emu.ScrollbackLine(y)
		authCells := make([]uv.Cell, len(authLine))
		copy(authCells, authLine)
		authText := strings.TrimRight(plainText(authCells), " ")
		assert.Equal(t, authText, simSB[y], "scrollback line %d", y)
	}
}

func TestDiffEmitter_AltScreenSkipsScrollback(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	m.Write([]byte("\x1b[?1049h")) // alt screen
	_, need := e.Emit(m)
	require.True(t, need) // flip → keyframe
	e.Prime(m)
	m.Write([]byte("APP"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "APP")
}

func TestDiffEmitter_ModeToggleForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?2004h")) // bracketed paste ON
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b[?2004h")

	m.Write([]byte("\x1b[?2004l")) // OFF again
	data, _ = e.Emit(m)
	assert.Contains(t, string(data), "\x1b[?2004l")
}

func TestDiffEmitter_TitleChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b]0;my-title\x07"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b]0;my-title\x07")
}

func TestDiffEmitter_UnchangedChromeEmitsNothing(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	m.Write([]byte("\x1b[?2004h\x1b]0;t\x07"))
	e := NewDiffEmitter()
	e.Prime(m)
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Empty(t, data, "already-primed chrome must not re-emit")
}

func TestDiffEmitter_ScrollRegionChangeNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 10, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[2;5r")) // DECSTBM
	_, need := e.Emit(m)
	assert.True(t, need, "a new scroll region invalidates the absolute-CUP diff base")
}

func TestDiffEmitter_OriginModeChangeNeedsKeyframe(t *testing.T) {
	m, _ := newTestModel(t, 10, 5)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?6h")) // DECOM origin mode
	_, need := e.Emit(m)
	assert.True(t, need, "origin mode invalidates the absolute-CUP diff base")
}

func TestDiffEmitter_CursorVisibilityChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?25l")) // hide cursor
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.ResetMode(ansi.DECMode(25)))
}

func TestDiffEmitter_CursorStyleChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	vm := m.(*vtModel)
	vm.shadow.cursorShapeSet = true
	vm.shadow.cursorShape = vt.CursorBar
	vm.shadow.cursorBlink = true
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.SetCursorStyle(5))
}

func TestDiffEmitter_DefaultColorChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	vm := m.(*vtModel)
	vm.shadow.setDefaultColor(0, color.RGBA{R: 0xff, A: 0xff})
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b]10;rgb:ffff/0000/0000\x1b\\")
}

// The following tests cover the set→unset direction of chrome state: an app
// that sets a default colour or cursor shape and then explicitly resets it
// must have the client's corresponding state corrected too, not left stale
// until an unrelated keyframe.
//
// The colour tests drive the shadow fields directly (vm.shadow.fgSet = false,
// mirroring how TestDiffEmitter_CursorStyleChangeForwarded and
// TestDiffEmitter_DefaultColorChangeForwarded already isolate the diff layer
// from the model's OSC plumbing) rather than writing a real OSC 110/111/112
// through m.Write: the pinned x/vt commit's Set*Color(nil) substitutes its
// internal defaultFg/defaultBg (color.White/color.Black, set in NewEmulator)
// for the nil argument before invoking the ForegroundColor/BackgroundColor
// callback, so an app-issued OSC 110/111/112 never actually delivers nil to
// vtModel.observeDefaultColor at this pin — clearDefaultColor is reachable
// only by other means (this is an upstream-plumbing gap outside diff.go's
// scope; the diff emitter must still forward the set→unset transition
// correctly whenever the shadow does clear the flag).

func TestDiffEmitter_ForegroundColorResetForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	vm := m.(*vtModel)
	vm.shadow.setDefaultColor(0, color.RGBA{R: 0xff, A: 0xff})
	e.Prime(m)

	vm.shadow.fg, vm.shadow.fgSet = nil, false
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.ResetForegroundColor, "fg set→unset must emit the OSC 110 reset")
}

func TestDiffEmitter_BackgroundColorResetForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	vm := m.(*vtModel)
	vm.shadow.setDefaultColor(1, color.RGBA{G: 0xff, A: 0xff})
	e.Prime(m)

	vm.shadow.bg, vm.shadow.bgSet = nil, false
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.ResetBackgroundColor, "bg set→unset must emit the OSC 111 reset")
}

func TestDiffEmitter_CursorColorResetForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	vm := m.(*vtModel)
	vm.shadow.setDefaultColor(2, color.RGBA{B: 0xff, A: 0xff})
	e.Prime(m)

	vm.shadow.cursorColor, vm.shadow.cursorColorSet = nil, false
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.ResetCursorColor, "cursor-colour set→unset must emit the OSC 112 reset")
}

func TestDiffEmitter_CursorShapeResetForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	vm := m.(*vtModel)
	vm.shadow.cursorShapeSet = true
	vm.shadow.cursorShape = vt.CursorBar
	vm.shadow.cursorBlink = true
	e := NewDiffEmitter()
	e.Prime(m)

	m.OnForegroundReset() // clears cursorShapeSet via resetTransientModes
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.SetCursorStyle(0), "cursor-shape set→unset must emit the DECSCUSR default form")
}
