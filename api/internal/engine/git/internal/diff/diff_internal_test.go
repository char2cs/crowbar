// Package diff_internal_test exercises private parsing helpers that cannot be
// triggered via real git output from the external test package.
package diff

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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

func TestParseDiffGitPath_PathWithSpaces(t *testing.T) {
	result := parseDiffGitPath("diff --git a/my file.go b/my file.go")
	assert.Equal(t, "my file.go", result)
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
	lines := []gitdomain.DiffLine{{LineType: gitdomain.DiffLineContext, Content: "ctx"}}
	result := headerForHunk(lines, -1)
	assert.Equal(t, "", result)
}

func TestHeaderForHunk_IndexTooLarge(t *testing.T) {
	lines := []gitdomain.DiffLine{{LineType: gitdomain.DiffLineHeader, Content: "@@ -1 +1 @@"}}
	result := headerForHunk(lines, 5)
	assert.Equal(t, "", result)
}

func TestHeaderForHunk_NonHeaderAtIndex(t *testing.T) {
	lines := []gitdomain.DiffLine{
		{LineType: gitdomain.DiffLineContext, Content: "context line"},
	}
	result := headerForHunk(lines, 0)
	assert.Equal(t, "", result)
}

// --- buildHunkLines: empty rawLines ---

func TestBuildHunkLines_Empty(t *testing.T) {
	result := buildHunkLines(nil, "file.go")
	assert.Nil(t, result)
}

// --- buildDiffLine: empty raw string and unknown prefix ---

func TestBuildDiffLine_EmptyRaw(t *testing.T) {
	old, new_ := 1, 1
	result := buildDiffLine("", "hunkID", &old, &new_)
	assert.Equal(t, gitdomain.DiffLine{}, result)
}

func TestBuildDiffLine_UnknownPrefix(t *testing.T) {
	old, new_ := 1, 1
	result := buildDiffLine("\\no newline", "hunkID", &old, &new_)
	assert.Equal(t, gitdomain.DiffLineContext, result.LineType)
	assert.Equal(t, "\\no newline", result.Content)
}

// --- hunkHeader: no DiffLineHeader in slice ---

func TestHunkHeader_NoHeaderLine_ReturnsFallback(t *testing.T) {
	lines := []gitdomain.DiffLine{
		{LineType: gitdomain.DiffLineContext, Content: " context"},
		{LineType: gitdomain.DiffLineAdded, Content: "+added"},
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

// --- parseCommitMeta: NUL-delimited format ---

func TestParseCommitMeta_MissingSeparators(t *testing.T) {
	result := parseCommitMeta("some output without separators")
	assert.Empty(t, result.CommitHash)
	assert.Empty(t, result.CommitAuthor)
}

func TestParseCommitMeta_ValidOutput(t *testing.T) {
	// Format: hash NUL subject NUL body NUL author NUL date
	output := "abc123\x00subject\x00body\x00John Doe\x002024-01-01T12:00:00Z\n"
	result := parseCommitMeta(output)
	assert.Equal(t, "abc123", result.CommitHash)
	assert.Equal(t, "subject", result.CommitMessage)
	assert.Equal(t, "John Doe", result.CommitAuthor)
	assert.NotNil(t, result.CommitDate)
}

// --- parseHunkHeader: no space/comma after old or new start number ---

func TestParseHunkHeader_EndNoSpaceOrComma_OldSide(t *testing.T) {
	// "-5" at end of string — IndexAny returns -1 so end = len(rest)
	old, new_ := parseHunkHeader("@@ -5")
	assert.Equal(t, 5, old)
	assert.Equal(t, 1, new_)
}

func TestParseHunkHeader_EndNoSpaceOrComma_NewSide(t *testing.T) {
	// "+9" at end of string — IndexAny returns -1 so end2 = len(rest2)
	old, new_ := parseHunkHeader("@@ -1,3 +9")
	assert.Equal(t, 1, old)
	assert.Equal(t, 9, new_)
}

// --- applyNewPath: FilePath fallback when parseDiffGitPath returns "" ---

func TestApplyNewPath_FilePath_FallbackFromNewPath(t *testing.T) {
	// Simulate a diff section where the "diff --git" line is malformed,
	// so parseDiffGitPath returns "". The "+++ b/" line must fill FilePath.
	section := "diff --git malformed\n--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n+package main\n"
	f := parseFileSection(context.Background(), "", section)
	assert.Equal(t, "foo.go", f.FilePath)
}

// --- buildPatch: deleted file branch ---

func TestBuildPatch_DeletedFile(t *testing.T) {
	f := &gitdomain.FileDiff{
		FilePath:  "gone.go",
		OldPath:   "gone.go",
		IsDeleted: true,
	}
	hunk := &gitdomain.Hunk{
		HunkID:    "testhunk",
		Header:    "@@ -1 +0,0 @@",
		StartLine: 0,
		EndLine:   0,
	}
	lines := []gitdomain.DiffLine{
		{LineType: gitdomain.DiffLineHeader, Content: "@@ -1 +0,0 @@", HunkID: "testhunk"},
	}
	patch := buildPatch(f, hunk, lines)
	assert.Contains(t, patch, "deleted file mode 100644")
	assert.Contains(t, patch, "--- a/gone.go")
	assert.Contains(t, patch, "+++ /dev/null")
}

// --- OldPath/NewPath should not be /dev/null ---

func TestParseFileSection_NewFile_NoDevNullInOldPath(t *testing.T) {
	section := "diff --git a/new.go b/new.go\nnew file mode 100644\nindex 0000000..abc1234\n--- /dev/null\n+++ b/new.go\n@@ -0,0 +1 @@\n+package main\n"
	f := parseFileSection(context.Background(), "", section)
	assert.True(t, f.IsNew)
	assert.Equal(t, "", f.OldPath, "OldPath must not be set to /dev/null")
	assert.Equal(t, "new.go", f.NewPath)
}

func TestParseFileSection_DeletedFile_NoDevNullInNewPath(t *testing.T) {
	section := "diff --git a/old.go b/old.go\ndeleted file mode 100644\nindex abc1234..0000000\n--- a/old.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-package main\n"
	f := parseFileSection(context.Background(), "", section)
	assert.True(t, f.IsDeleted)
	assert.Equal(t, "", f.NewPath, "NewPath must not be set to /dev/null")
	assert.Equal(t, "old.go", f.OldPath)
}
