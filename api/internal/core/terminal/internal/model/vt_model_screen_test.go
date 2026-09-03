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
