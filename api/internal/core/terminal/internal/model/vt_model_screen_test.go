package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ ScreenReader = (*vtModel)(nil)

func TestVtModel_ScreenText_RendersWrittenTextAtRowCol(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.Write([]byte("hello"))
	m.Write([]byte("\x1b[3;10Hworld"))

	lines := strings.Split(m.ScreenText(), "\n")
	require.Len(t, lines, 5, "one line per Rows()")
	assert.Equal(t, "hello", lines[0], "row 1 holds the first write, left-anchored at col 1")
	assert.Equal(t, strings.Repeat(" ", 9)+"world", lines[2],
		"row 3 holds the CUP-positioned write at col 10 (9 leading blank cells)")
}

func TestVtModel_ScreenText_TrimsTrailingBlanksButPreservesInteriorSpaces(t *testing.T) {
	m := newVTModel(20, 3, 100)
	m.Write([]byte("a  b"))

	lines := strings.Split(m.ScreenText(), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "a  b", lines[0],
		"interior double space preserved; trailing nil cells trimmed instead of padded to 20 cols")
}

func TestVtModel_ScreenText_UntouchedScreenIsBlankLinesMatchingRowCount(t *testing.T) {
	m := newVTModel(10, 5, 100)

	lines := strings.Split(m.ScreenText(), "\n")
	require.Len(t, lines, m.Rows(), "row count must match Rows() even with nothing painted")
	for i, line := range lines {
		assert.Empty(t, line, "untouched row %d must render blank, not garbage", i)
	}
}

func TestVtModel_ScreenText_StripsSGRButKeepsLetters(t *testing.T) {
	m := newVTModel(20, 3, 100)
	m.Write([]byte("\x1b[31mred\x1b[0m text"))

	text := m.ScreenText()
	assert.NotContains(t, text, "\x1b", "no raw escape byte may reach the plain-text render")
	assert.Contains(t, text, "red text", "the SGR-wrapped letters must still render")
}

var _ ScreenStateReader = (*vtModel)(nil)

func TestVtModel_ScreenState_BracketedPasteFollowsMode2004(t *testing.T) {
	m := newVTModel(20, 5, 100)
	assert.False(t, m.ScreenState().BracketedPaste, "off until an app asks for it")

	m.Write([]byte("\x1b[?2004h"))
	assert.True(t, m.ScreenState().BracketedPaste)

	m.Write([]byte("\x1b[?2004l"))
	assert.False(t, m.ScreenState().BracketedPaste)

	m.Write([]byte("\x1b[?2004h"))
	m.OnForegroundReset()
	assert.False(t, m.ScreenState().BracketedPaste, "a dead app's paste mode must not outlive it")
}

func TestVtModel_ScreenState_CursorSitsAfterTheWrittenPrompt(t *testing.T) {
	m := newVTModel(20, 5, 100)
	m.Write([]byte("\x1b[3;1H❯ "))

	st := m.ScreenState()
	assert.Equal(t, 2, st.CursorRow, "0-based row of the prompt")
	assert.Equal(t, 2, st.CursorCol, "prompt glyph and space are two cells")
	assert.Equal(t, m.ScreenText(), st.Text, "one read returns the same text ScreenText does")
}

func TestVtModel_ScreenState_GhostTextLeavesTheCursorBehindIt(t *testing.T) {
	m := newVTModel(30, 3, 100)
	m.Write([]byte("❯ \x1b[2mrun the tests\x1b[0m\x1b[13D"))

	st := m.ScreenState()
	assert.Equal(t, 0, st.CursorRow)
	assert.Equal(t, 2, st.CursorCol, "ghost text paints after the cursor, it does not move it")
	assert.Contains(t, st.Text, "❯ run the tests", "the ghost text is on the row")

	m.Write([]byte("fix"))
	assert.Equal(t, 5, m.ScreenState().CursorCol, "a typed draft pushes the cursor past the prompt")
}

func TestVtModel_ScreenState_MarksFaintCells(t *testing.T) {
	m := newVTModel(20, 3, 100)
	m.Write([]byte("❯ \x1b[2mghost\x1b[22m hi"))

	st := m.ScreenState()
	require.Len(t, st.Faint, 3, "one mask per visible row")
	require.Len(t, st.Faint[0], 20, "one cell per column")
	assert.Equal(t, []bool{false, false, true, true, true, true, true, false, false, false}, st.Faint[0][:10])
	assert.NotContains(t, st.Faint[1], true, "an unpainted row is not faint")
}

func TestVtModel_ScreenState_WideCharacterCountsTwoCells(t *testing.T) {
	m := newVTModel(20, 3, 100)
	m.Write([]byte("日本"))

	assert.Equal(t, 4, m.ScreenState().CursorCol, "each double-width character occupies two cells")
}
