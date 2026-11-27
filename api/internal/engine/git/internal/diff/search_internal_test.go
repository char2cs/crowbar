package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchHunkStarts_TooFewFields(t *testing.T) {
	_, _, ok := searchHunkStarts([]byte("@@ -1,5"))
	assert.False(t, ok)
}

func TestSearchHunkStarts_WrongPrefixes(t *testing.T) {
	_, _, ok := searchHunkStarts([]byte("@@ 1,5 +1,5 @@"))
	assert.False(t, ok)
}

func TestSearchRangeStart_NonNumeric(t *testing.T) {
	_, ok := searchRangeStart("x,5")
	assert.False(t, ok)
}

func TestStartHunk_MalformedHeaderLeavesStateUnchanged(t *testing.T) {
	s := &diffSearch{}

	s.startHunk([]byte("@@ garbage @@"))

	assert.False(t, s.inHunk)
	assert.Zero(t, s.oldLine)
	assert.Zero(t, s.newLine)
}

func TestSearchUnquotePath_InvalidEscapeReturnsRawInput(t *testing.T) {
	got := searchUnquotePath(`"unterminated`)
	assert.Equal(t, `"unterminated`, got)
}

func TestDiffSearch_BodyBlankLineAdvancesBothCountersWithoutRecording(t *testing.T) {
	matchCalled := false
	s := &diffSearch{
		oldLine: 5,
		newLine: 7,
		match:   func([]byte) bool { matchCalled = true; return true },
	}

	s.body([]byte{})

	assert.Equal(t, 6, s.oldLine)
	assert.Equal(t, 8, s.newLine)
	assert.Empty(t, s.hits)
	assert.False(t, matchCalled, "a blank line carries no content to match against")
}

func TestDiffSearch_FinishPropagatesNonEOFError(t *testing.T) {
	s := &diffSearch{match: func([]byte) bool { return false }}

	err := s.finish([]byte("trailing"), assert.AnError)

	assert.ErrorIs(t, err, assert.AnError)
}
