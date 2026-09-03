// Package diff (internal tests) exercises search.go's private parsing helpers
// with malformed input real git output never actually produces.
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

// TestStartHunk_MalformedHeaderLeavesStateUnchanged covers startHunk's !ok
// branch: a header searchHunkStarts cannot parse must not flip the scanner
// into hunk mode with garbage counters.
func TestStartHunk_MalformedHeaderLeavesStateUnchanged(t *testing.T) {
	s := &diffSearch{}

	s.startHunk([]byte("@@ garbage @@"))

	assert.False(t, s.inHunk)
	assert.Zero(t, s.oldLine)
	assert.Zero(t, s.newLine)
}

// TestSearchUnquotePath_InvalidEscapeReturnsRawInput covers the one path a real
// git C-quoted path cannot reach: a string that opens with a quote but is not
// valid Go-string-literal syntax must be passed through unchanged rather than
// dropped.
func TestSearchUnquotePath_InvalidEscapeReturnsRawInput(t *testing.T) {
	got := searchUnquotePath(`"unterminated`)
	assert.Equal(t, `"unterminated`, got)
}

// TestDiffSearch_BodyBlankLineAdvancesBothCountersWithoutRecording covers the
// truly-blank-context-line case (git's diff.suppressBlankEmpty writes a bare
// line for a blank unchanged line rather than a lone space): it must advance
// both line counters like any other context line, and — since it carries no
// content — never call match or record a hit.
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

// TestDiffSearch_FinishPropagatesNonEOFError covers finish()'s final branch: a
// genuine read error (not io.EOF) must be returned, not swallowed, even after
// any trailing partial line is consumed.
func TestDiffSearch_FinishPropagatesNonEOFError(t *testing.T) {
	s := &diffSearch{match: func([]byte) bool { return false }}

	err := s.finish([]byte("trailing"), assert.AnError)

	assert.ErrorIs(t, err, assert.AnError)
}
