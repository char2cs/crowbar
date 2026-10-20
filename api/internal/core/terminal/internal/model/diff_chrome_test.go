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
	m.Write([]byte("\x1b[?2004h")) // bracketed paste ON
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b[?2004h")

	m.Write([]byte("\x1b[?2004l")) // OFF again
	data, _ = e.Emit(m)
	assert.Contains(t, string(data), "\x1b[?2004l")
}

// TestDiffEmitter_TitleChangeForwarded pins the title delta to the
// serializer's exact OSC 2 ST form — never OSC 0. OSC 0 sets BOTH title and
// icon; emitting it for a title-only change would clobber the client's icon
// name, and since shadow.iconName didn't change no corrective OSC 1 would
// fire, leaving the icon wrong until the next keyframe.
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

// TestDiffEmitter_IconNameChangeForwarded pins M3: an app that sets ONLY the icon
// name (OSC 1) must have that change streamed as a diff, in the serializer's exact
// OSC 1 ST form — previously the diff emitter tracked only the title and silently
// dropped a lone icon-name change.
func TestDiffEmitter_IconNameChangeForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b]1;my-icon\x07")) // OSC 1 icon name only
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b]1;my-icon\x1b\\",
		"a lone icon-name (OSC 1) change must stream in the serializer's OSC 1 ST form")
	assert.NotContains(t, string(data), "\x1b]0;", "an OSC 1 change must not emit an OSC 0 (which would clobber the title)")
}

// TestDiffEmitter_TitleOnlyChangeDoesNotClobberIcon is the red-first
// regression for the OSC-0-clobbers-icon bug: prime with an icon name set via
// OSC 1, then change ONLY the title via OSC 2. The delta must carry the title
// as OSC 2 and must NOT contain any OSC 0/OSC 1 write — an OSC 0 would
// overwrite the (unchanged) icon with the title text, and a spurious OSC 1
// would be redundant since the icon never changed.
func TestDiffEmitter_TitleOnlyChangeDoesNotClobberIcon(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	m.Write([]byte("\x1b]1;my-icon\x07")) // set the icon before priming
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b]2;new-title\x07")) // title-only change
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

// TestDiffEmitter_AutowrapModeFlipForwarded pins M5(a): a tracked DEC private
// mode that is NOT in serializedModeOrder — DECAWM autowrap (mode 7) — must still
// stream its flip, via writeModeDelta's "extras" path (the sorted append of modes
// outside the ordered list).
func TestDiffEmitter_AutowrapModeFlipForwarded(t *testing.T) {
	m, _ := newTestModel(t, 10, 3)
	e := NewDiffEmitter()
	e.Prime(m)
	m.Write([]byte("\x1b[?7l")) // DECAWM autowrap OFF
	data, need := e.Emit(m)
	require.False(t, need)
	assert.Contains(t, string(data), "\x1b[?7l",
		"an autowrap (mode 7) flip must stream through writeModeDelta's extras path")
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
