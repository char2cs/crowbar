package diff

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- parseHunkShape: malformed headers real git never emits ---

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

// --- parseHunkRange: malformed fields ---

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

// --- outlineScan.line: a blank line outside any open file entry ---

func TestOutlineScan_LineIgnoresBlankLineBeforeAnyFile(t *testing.T) {
	s := &outlineScan{}
	s.line([]byte{})
	assert.False(t, s.open)
	assert.Empty(t, s.files)
}

// --- outlineScan.step: EOF that still carries a final, unterminated line ---

func TestOutlineScan_StepReturnsDoneOnEOFWithTrailingLine(t *testing.T) {
	s := &outlineScan{}
	r := bufio.NewReader(strings.NewReader("diff --git a/f b/f"))

	done, err := s.step(r)

	require.NoError(t, err)
	assert.True(t, done, "EOF must still be reported done even when the final line had content")
	assert.True(t, s.open, "the trailing line must still be processed as a header before EOF is returned")
}

// --- outlineScan.run: a genuine (non-EOF) read error must propagate ---

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

// --- outlineScan.body: a truly blank line inside a hunk ---

// TestOutlineScan_BodyBlankLineDecrementsBothCounters pins the case documented
// on body: with diff.suppressBlankEmpty set, git writes a blank context line
// (no leading space) rather than a lone-space line, and it must still consume
// one line from each side's remaining count like any other context line.
func TestOutlineScan_BodyBlankLineDecrementsBothCounters(t *testing.T) {
	s := &outlineScan{oldRemaining: 2, newRemaining: 2}

	s.body([]byte{})

	assert.Equal(t, 1, s.oldRemaining)
	assert.Equal(t, 1, s.newRemaining)
}

// TestOutlineScan_BodyMiscountedHunkResyncsOnNextHeader covers the
// self-correction path documented on body: a hunk body line that starts with
// none of +/-/space/\\ means the header lied about the hunk's size, so the
// counters are dropped and the line is re-dispatched as a fresh header rather
// than being swallowed as bogus hunk content.
func TestOutlineScan_BodyMiscountedHunkResyncsOnNextHeader(t *testing.T) {
	s := &outlineScan{open: true, oldRemaining: 3, newRemaining: 3}

	s.body([]byte("diff --git a/next b/next"))

	assert.Zero(t, s.oldRemaining, "a resync must abandon the stale hunk counts")
	assert.Zero(t, s.newRemaining)
	assert.Equal(t, "next", s.current.Path, "the miscounted line must be reparsed as the next file's header")
}

// --- outlineScan.header: a header-shaped line before any "diff --git" ---

// TestOutlineScan_HeaderIgnoredBeforeAnyFileOpened covers the guard against a
// line that looks like part of a file header (e.g. a "+++ " line) arriving
// before the scan has ever seen a "diff --git" line to open a file for.
func TestOutlineScan_HeaderIgnoredBeforeAnyFileOpened(t *testing.T) {
	s := &outlineScan{}

	s.header([]byte("+++ b/somewhere.txt"))

	assert.False(t, s.open)
	assert.Empty(t, s.files)
}

// --- outlineScan.startHunk: a "@@ " line whose shape fails to parse ---

// TestOutlineScan_StartHunkMalformedShapeIsSkipped covers startHunk's guard: a
// hunk header git's own writer never emits malformed, so a state machine that
// scans by prefix alone could otherwise be fed one and desync. The hunk must
// simply be skipped, not counted or appended.
func TestOutlineScan_StartHunkMalformedShapeIsSkipped(t *testing.T) {
	s := &outlineScan{open: true}

	s.startHunk([]byte("@@ garbage"))

	assert.Zero(t, s.hunksSeen)
	assert.Empty(t, s.current.Hunks)
}

// --- diffGitPath / trimSidePrefix / unquotePath: fallbacks real git never
// triggers, guarded here directly since the format they defend against
// (an unquoted "diff --git" line with no " b/" separator, or an unpaired
// leading quote) cannot be produced by a real git subprocess. ---

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
