package model

import (
	"image/color"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffEmitter_ModeToggleForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?2004h"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b[?2004h")

	m.Write([]byte("\x1b[?2004l"))
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
	assert.Contains(t, string(data), "\x1b]2;my-title\x1b\\",
		"a title change must stream in the serializer's OSC 2 ST form")
	assert.NotContains(t, string(data), "\x1b]0;", "a title change must not emit an OSC 0 (which would clobber the icon)")
}

func TestDiffEmitter_IconNameChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b]1;my-icon\x07"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b]1;my-icon\x1b\\",
		"a lone icon-name (OSC 1) change must stream in the serializer's OSC 1 ST form")
	assert.NotContains(t, string(data), "\x1b]0;", "an OSC 1 change must not emit an OSC 0 (which would clobber the title)")
}

func TestDiffEmitter_TitleOnlyChangeDoesNotClobberIcon(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	m.Write([]byte("\x1b]1;my-icon\x07"))
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b]2;new-title\x07"))
	data, need := e.Emit(m)
	require.False(t, need)
	got := string(data)
	assert.Contains(t, got, "\x1b]2;new-title\x1b\\",
		"a title-only change must stream in the serializer's OSC 2 ST form")
	assert.NotContains(t, got, "\x1b]0;", "a title-only change must never emit an OSC 0 (it would clobber the icon)")
	assert.NotContains(t, got, "\x1b]1;", "the icon did not change, so no OSC 1 icon-name write should be emitted")
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

func TestDiffEmitter_AutowrapModeFlipForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?7l"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b[?7l",
		"an autowrap (mode 7) flip must stream through writeModeDelta's extras path")
}

func TestDiffEmitter_CursorVisibilityChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?25l"))
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.ResetMode(ansi.DECMode(25)))
}

func TestDiffEmitter_CursorStyleChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	vm := asVT(t, m)
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
	vm := asVT(t, m)
	vm.shadow.setDefaultColor(0, color.RGBA{R: 0xff, A: 0xff})
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b]10;rgb:ffff/0000/0000\x1b\\")
}

func TestDiffEmitter_ForegroundColorResetForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	vm := asVT(t, m)
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
	vm := asVT(t, m)
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
	vm := asVT(t, m)
	vm.shadow.setDefaultColor(2, color.RGBA{B: 0xff, A: 0xff})
	e.Prime(m)

	vm.shadow.cursorColor, vm.shadow.cursorColorSet = nil, false
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.ResetCursorColor, "cursor-colour set→unset must emit the OSC 112 reset")
}

func TestDiffEmitter_CursorShapeResetForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	vm := asVT(t, m)
	vm.shadow.cursorShapeSet = true
	vm.shadow.cursorShape = vt.CursorBar
	vm.shadow.cursorBlink = true
	e := NewDiffEmitter()
	e.Prime(m)

	m.OnForegroundReset()
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), ansi.SetCursorStyle(0), "cursor-shape set→unset must emit the DECSCUSR default form")
}
