package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ScreenReader is implemented by *vtModel via a guarded type assertion at every call
// site (Session.ScreenText), exactly like ThemeAware and ModelHealth. There is no
// compiler error if that assertion silently starts failing — the session would just
// degrade to "nothing to see" forever — so pin the pairing here as a compile-time
// assertion rather than relying on some other test noticing the type switch always
// misses.
var _ ScreenReader = (*vtModel)(nil)

// TestVtModel_ScreenText_RendersWrittenTextAtRowCol proves ScreenText places written
// text at the row/column the cursor was actually positioned at, not merely "somewhere
// on screen" — a serializer bug that scrambled rows or columns could still produce a
// plausible-looking blob of text and slip past a substring-only assertion.
func TestVtModel_ScreenText_RendersWrittenTextAtRowCol(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.Write([]byte("hello"))
	m.Write([]byte("\x1b[3;10Hworld")) // CUP: row 3, col 10 (both 1-indexed on the wire)

	lines := strings.Split(m.ScreenText(), "\n")
	require.Len(t, lines, 5, "one line per Rows()")
	assert.Equal(t, "hello", lines[0], "row 1 holds the first write, left-anchored at col 1")
	assert.Equal(t, strings.Repeat(" ", 9)+"world", lines[2],
		"row 3 holds the CUP-positioned write at col 10 (9 leading blank cells)")
}

// TestVtModel_ScreenText_TrimsTrailingBlanksButPreservesInteriorSpaces proves the
// trailing-blank trim documented on ScreenText (so a mostly-empty screen does not cost
// cols bytes per row) stops exactly at the last non-blank cell, rather than also eating
// whitespace the app deliberately put in the MIDDLE of a line.
func TestVtModel_ScreenText_TrimsTrailingBlanksButPreservesInteriorSpaces(t *testing.T) {
	m := newVTModel(20, 3, 100)
	m.Write([]byte("a  b")) // interior double space, then nothing — cols 5..20 are nil cells

	lines := strings.Split(m.ScreenText(), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "a  b", lines[0],
		"interior double space preserved; trailing nil cells trimmed instead of padded to 20 cols")
}

// TestVtModel_ScreenText_UntouchedScreenIsBlankLinesMatchingRowCount proves a model
// that has never been written renders as exactly Rows() blank lines: nil cells must
// render as spaces (then trim to empty), not as garbage from unzeroed cell state, and
// the row count must match Rows() so a line-oriented caller never sees a truncated grid.
func TestVtModel_ScreenText_UntouchedScreenIsBlankLinesMatchingRowCount(t *testing.T) {
	m := newVTModel(10, 5, 100)

	lines := strings.Split(m.ScreenText(), "\n")
	require.Len(t, lines, m.Rows(), "row count must match Rows() even with nothing painted")
	for i, line := range lines {
		assert.Empty(t, line, "untouched row %d must render blank, not garbage", i)
	}
}

// TestVtModel_ScreenText_StripsSGRButKeepsLetters proves ScreenText renders CONTENT
// ONLY, per its doc comment: an app colouring its output must not leak raw SGR escape
// bytes into a caller that treats the result as plain text (the daemon's own inspection
// of what a hosted CLI is showing), while the letters the escapes were wrapping around
// must still survive intact.
func TestVtModel_ScreenText_StripsSGRButKeepsLetters(t *testing.T) {
	m := newVTModel(20, 3, 100)
	m.Write([]byte("\x1b[31mred\x1b[0m text"))

	text := m.ScreenText()
	assert.NotContains(t, text, "\x1b", "no raw escape byte may reach the plain-text render")
	assert.Contains(t, text, "red text", "the SGR-wrapped letters must still render")
}
