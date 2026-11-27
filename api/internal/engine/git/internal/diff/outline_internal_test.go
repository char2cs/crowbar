package diff

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHunkShape_MissingClosingAtAt(t *testing.T) {
	_, ok := parseHunkShape("-1,5 +1,5")
	assert.False(t, ok)
}

func TestParseHunkShape_WrongFieldCount(t *testing.T) {
	_, ok := parseHunkShape("-1,5 @@")
	assert.False(t, ok)
}

func TestParseHunkShape_BadOldRange(t *testing.T) {
	_, ok := parseHunkShape("garbage +1,5 @@")
	assert.False(t, ok)
}

func TestParseHunkShape_BadNewRange(t *testing.T) {
	_, ok := parseHunkShape("-1,5 garbage @@")
	assert.False(t, ok)
}

func TestParseHunkRange_TooShort(t *testing.T) {
	_, _, ok := parseHunkRange("-", '-')
	assert.False(t, ok)
}

func TestParseHunkRange_WrongSign(t *testing.T) {
	_, _, ok := parseHunkRange("+1,5", '-')
	assert.False(t, ok)
}

func TestParseHunkRange_NonNumericStart(t *testing.T) {
	_, _, ok := parseHunkRange("-x,5", '-')
	assert.False(t, ok)
}

func TestParseHunkRange_NonNumericCount(t *testing.T) {
	_, _, ok := parseHunkRange("-1,x", '-')
	assert.False(t, ok)
}

func TestParseHunkRange_NoCountDefaultsToOne(t *testing.T) {
	start, count, ok := parseHunkRange("-7", '-')
	require.True(t, ok)
	assert.Equal(t, 7, start)
	assert.Equal(t, 1, count)
}

func TestOutlineScan_LineIgnoresBlankLineBeforeAnyFile(t *testing.T) {
	s := &outlineScan{}
	s.line([]byte{})
	assert.False(t, s.open)
	assert.Empty(t, s.files)
}

func TestOutlineScan_StepReturnsDoneOnEOFWithTrailingLine(t *testing.T) {
	s := &outlineScan{}
	r := bufio.NewReader(strings.NewReader("diff --git a/f b/f"))

	done, err := s.step(r)

	require.NoError(t, err)
	assert.True(t, done, "EOF must still be reported done even when the final line had content")
	assert.True(t, s.open, "the trailing line must still be processed as a header before EOF is returned")
}

type erroringReader struct{}

func (erroringReader) Read([]byte) (int, error) {
	return 0, assert.AnError
}

func TestOutlineScan_RunPropagatesNonEOFReadError(t *testing.T) {
	s := &outlineScan{}
	r := bufio.NewReader(erroringReader{})

	err := s.run(r)

	assert.ErrorIs(t, err, assert.AnError)
}

func TestOutlineScan_BodyBlankLineDecrementsBothCounters(t *testing.T) {
	s := &outlineScan{oldRemaining: 2, newRemaining: 2}

	s.body([]byte{})

	assert.Equal(t, 1, s.oldRemaining)
	assert.Equal(t, 1, s.newRemaining)
}

func TestOutlineScan_BodyMiscountedHunkResyncsOnNextHeader(t *testing.T) {
	s := &outlineScan{open: true, oldRemaining: 3, newRemaining: 3}

	s.body([]byte("diff --git a/next b/next"))

	assert.Zero(t, s.oldRemaining, "a resync must abandon the stale hunk counts")
	assert.Zero(t, s.newRemaining)
	assert.Equal(t, "next", s.current.Path, "the miscounted line must be reparsed as the next file's header")
}

func TestOutlineScan_HeaderIgnoredBeforeAnyFileOpened(t *testing.T) {
	s := &outlineScan{}

	s.header([]byte("+++ b/somewhere.txt"))

	assert.False(t, s.open)
	assert.Empty(t, s.files)
}

func TestOutlineScan_StartHunkMalformedShapeIsSkipped(t *testing.T) {
	s := &outlineScan{open: true}

	s.startHunk([]byte("@@ garbage"))

	assert.Zero(t, s.hunksSeen)
	assert.Empty(t, s.current.Hunks)
}

func TestDiffGitPath_NoBSeparator_ReturnsRestUnchanged(t *testing.T) {
	got := diffGitPath("no-b-slash-marker-here")
	assert.Equal(t, "no-b-slash-marker-here", got)
}

func TestTrimSidePrefix_NoABPrefix_ReturnsUnchanged(t *testing.T) {
	got := trimSidePrefix("plain.txt")
	assert.Equal(t, "plain.txt", got)
}

func TestUnquotePath_MalformedQuoting_ReturnsRawUnchanged(t *testing.T) {
	got := unquotePath(`"unterminated`)
	assert.Equal(t, `"unterminated`, got)
}
