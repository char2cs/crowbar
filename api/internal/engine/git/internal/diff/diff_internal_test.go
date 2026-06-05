// Package diff_internal_test exercises private parsing helpers that cannot be
// triggered via real git output from the external test package.
package diff

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/stretchr/testify/assert"
)

// --- parseDiffGitPath: fewer than 4 fields ---

func TestParseDiffGitPath_ShortLine(t *testing.T) {
	result := parseDiffGitPath("diff --git")
	assert.Equal(t, "", result)
}

func TestParseDiffGitPath_ExactlyFourFields(t *testing.T) {
	result := parseDiffGitPath("diff --git a/foo b/foo")
	assert.Equal(t, "foo", result)
}

// --- parseIndexLine: malformed input ---

func TestParseIndexLine_EmptyAfterPrefix(t *testing.T) {
	old, new_ := parseIndexLine("index ")
	assert.Equal(t, "", old)
	assert.Equal(t, "", new_)
}

func TestParseIndexLine_NoDotDot(t *testing.T) {
	old, new_ := parseIndexLine("index abc123 100644")
	assert.Equal(t, "", old)
	assert.Equal(t, "", new_)
}

func TestParseIndexLine_Valid(t *testing.T) {
	old, new_ := parseIndexLine("index abc123..def456 100644")
	assert.Equal(t, "abc123", old)
	assert.Equal(t, "def456", new_)
}

// --- parseHunkHeader: missing markers ---

func TestParseHunkHeader_NoMinus(t *testing.T) {
	old, new_ := parseHunkHeader("@@ some line without markers @@")
	assert.Equal(t, 1, old)
	assert.Equal(t, 1, new_)
}

func TestParseHunkHeader_NoPlus(t *testing.T) {
	old, new_ := parseHunkHeader("@@ -5,3 no-plus @@")
	assert.Equal(t, 5, old)
	assert.Equal(t, 1, new_)
}

func TestParseHunkHeader_EndNoSpace(t *testing.T) {
	// old range has no space/comma after number
	old, new_ := parseHunkHeader("@@ -7 +9 @@")
	assert.Equal(t, 7, old)
	assert.Equal(t, 9, new_)
}

// --- headerForHunk: out-of-bounds and non-header type ---

func TestHeaderForHunk_NegativeIndex(t *testing.T) {
	lines := []domain.DiffLine{{LineType: domain.DiffLineContext, Content: "ctx"}}
	result := headerForHunk(lines, -1)
	assert.Equal(t, "", result)
}

func TestHeaderForHunk_IndexTooLarge(t *testing.T) {
	lines := []domain.DiffLine{{LineType: domain.DiffLineHeader, Content: "@@ -1 +1 @@"}}
	result := headerForHunk(lines, 5)
	assert.Equal(t, "", result)
}

func TestHeaderForHunk_NonHeaderAtIndex(t *testing.T) {
	lines := []domain.DiffLine{
		{LineType: domain.DiffLineContext, Content: "context line"},
	}
	result := headerForHunk(lines, 0)
	assert.Equal(t, "", result)
}

// --- buildHunkLines: empty rawLines ---

func TestBuildHunkLines_Empty(t *testing.T) {
	result := buildHunkLines(nil, "file.go", 0)
	assert.Nil(t, result)
}

// --- buildDiffLine: empty raw string and unknown prefix ---

func TestBuildDiffLine_EmptyRaw(t *testing.T) {
	old, new_ := 1, 1
	result := buildDiffLine("", "hunkID", &old, &new_)
	assert.Equal(t, domain.DiffLine{}, result)
}

func TestBuildDiffLine_UnknownPrefix(t *testing.T) {
	old, new_ := 1, 1
	result := buildDiffLine("\\no newline", "hunkID", &old, &new_)
	assert.Equal(t, domain.DiffLineContext, result.LineType)
	assert.Equal(t, "\\no newline", result.Content)
}

// --- hunkHeader: no DiffLineHeader in slice ---

func TestHunkHeader_NoHeaderLine_ReturnsFallback(t *testing.T) {
	lines := []domain.DiffLine{
		{LineType: domain.DiffLineContext, Content: " context"},
		{LineType: domain.DiffLineAdded, Content: "+added"},
	}
	result := hunkHeader(lines, "fallback-header")
	assert.Equal(t, "fallback-header", result)
}

// --- buildHunkBody: line with len 0 after empty/newline check ---

func TestBuildHunkBody_MixedLines(t *testing.T) {
	lines := []string{
		"+added line",
		"-removed line",
		" context line",
		"",
		"\\ No newline at end of file",
	}
	result := buildHunkBody(lines)
	assert.Contains(t, result, "+added line")
	assert.Contains(t, result, "-removed line")
	assert.Contains(t, result, " context line")
	assert.NotContains(t, result, "\\ No newline")
}

// --- isZeroSHA: all zeros ---

func TestIsZeroSHA_AllZeros(t *testing.T) {
	assert.True(t, isZeroSHA("0000000"))
	assert.True(t, isZeroSHA("0000000000000000000000000000000000000000"))
}

func TestIsZeroSHA_NonZero(t *testing.T) {
	assert.False(t, isZeroSHA("abc123"))
	assert.False(t, isZeroSHA("0000001"))
}

// --- parseCommitMeta: missing separators ---

func TestParseCommitMeta_MissingSeparators(t *testing.T) {
	result := parseCommitMeta("some output without separators")
	assert.Empty(t, result.CommitHash)
	assert.Empty(t, result.CommitAuthor)
}

func TestParseCommitMeta_ValidOutput(t *testing.T) {
	output := "abc123\nsubject\nbody\n---author---\nJohn Doe\n---date---\n2024-01-01T12:00:00Z\n"
	result := parseCommitMeta(output)
	assert.Equal(t, "abc123", result.CommitHash)
	assert.Equal(t, "subject", result.CommitMessage)
	assert.Equal(t, "John Doe", result.CommitAuthor)
	assert.NotNil(t, result.CommitDate)
}
