// Package conflicts internal tests exercise unexported helpers directly.
package conflicts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ---------------------------------------------------------------------------
// extractConflictBlocks
// ---------------------------------------------------------------------------

func TestExtractConflictBlocks_NoMarkers(
	t *testing.T,
) {
	blocks, err := extractConflictBlocks("no conflicts here\n")
	require.NoError(t, err)
	assert.Empty(t, blocks)
}

func TestExtractConflictBlocks_StandardMerge(
	t *testing.T,
) {
	content := "line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nline2\n"
	blocks, err := extractConflictBlocks(content)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "ours", blocks[0].oursRaw)
	assert.Equal(t, "theirs", blocks[0].theirsRaw)
	assert.Empty(t, blocks[0].baseRaw)
}

// TestExtractConflictBlocks_Diff3 exercises the ||||||| base-section branch.
func TestExtractConflictBlocks_Diff3(
	t *testing.T,
) {
	content := "line1\n<<<<<<< HEAD\nours\n||||||| base\nbase content\n=======\ntheirs\n>>>>>>> branch\nline2\n"
	blocks, err := extractConflictBlocks(content)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "ours", blocks[0].oursRaw)
	assert.Equal(t, "base content", blocks[0].baseRaw)
	assert.Equal(t, "theirs", blocks[0].theirsRaw)
}

// TestExtractConflictBlocks_MultipleHunks verifies two conflict hunks are parsed.
func TestExtractConflictBlocks_MultipleHunks(
	t *testing.T,
) {
	content := "<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> b\nmiddle\n<<<<<<< HEAD\nours2\n=======\ntheirs2\n>>>>>>> b\n"
	blocks, err := extractConflictBlocks(content)
	require.NoError(t, err)
	assert.Len(t, blocks, 2)
}

// TestExtractConflictBlocks_UnclosedMarker exercises the error path when a
// <<<<<<< has no matching >>>>>>>.
func TestExtractConflictBlocks_UnclosedMarker(
	t *testing.T,
) {
	content := "line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n"
	_, err := extractConflictBlocks(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unclosed conflict marker")
}

// ---------------------------------------------------------------------------
// extractLines
// ---------------------------------------------------------------------------

func TestExtractLines_Normal(
	t *testing.T,
) {
	content := "a\nb\nc\nd\ne"
	result := extractLines(content, 2, 4)
	assert.Equal(t, "b\nc", result)
}

// TestExtractLines_StartNegative exercises the start < 0 branch (startLine=0).
func TestExtractLines_StartNegative(
	t *testing.T,
) {
	content := "a\nb\nc"
	// startLine=0 → start = -1 → clamped to 0
	result := extractLines(content, 0, 2)
	assert.Equal(t, "a", result)
}

// TestExtractLines_EndBeyondLength exercises the end > len(lines) branch.
func TestExtractLines_EndBeyondLength(
	t *testing.T,
) {
	content := "a\nb\nc"
	// endLine=100 → end = 99 → clamped to len(lines)=3
	result := extractLines(content, 1, 100)
	assert.Equal(t, "a\nb\nc", result)
}

// TestExtractLines_StartGeEnd exercises the start >= end branch (returns "").
func TestExtractLines_StartGeEnd(
	t *testing.T,
) {
	content := "a\nb\nc"
	// startLine=3 → start=2, endLine=2 → end=1, start >= end
	result := extractLines(content, 3, 2)
	assert.Equal(t, "", result)
}

// ---------------------------------------------------------------------------
// resolvedText
// ---------------------------------------------------------------------------

func TestResolvedText_Ours(
	t *testing.T,
) {
	b := &conflictBlock{oursRaw: "ours", theirsRaw: "theirs"}
	got, err := resolvedText(b, gitdomain.ConflictResolutionOurs, "")
	require.NoError(t, err)
	assert.Equal(t, "ours", got)
}

func TestResolvedText_Theirs(
	t *testing.T,
) {
	b := &conflictBlock{oursRaw: "ours", theirsRaw: "theirs"}
	got, err := resolvedText(b, gitdomain.ConflictResolutionTheirs, "")
	require.NoError(t, err)
	assert.Equal(t, "theirs", got)
}

func TestResolvedText_Both(
	t *testing.T,
) {
	b := &conflictBlock{oursRaw: "ours", theirsRaw: "theirs"}
	got, err := resolvedText(b, gitdomain.ConflictResolutionBoth, "")
	require.NoError(t, err)
	assert.Equal(t, "ours\ntheirs", got)
}

func TestResolvedText_Custom(
	t *testing.T,
) {
	b := &conflictBlock{oursRaw: "ours", theirsRaw: "theirs"}
	got, err := resolvedText(b, gitdomain.ConflictResolutionCustom, "custom text")
	require.NoError(t, err)
	assert.Equal(t, "custom text", got)
}

func TestResolvedText_Default(
	t *testing.T,
) {
	b := &conflictBlock{oursRaw: "ours"}
	// Unknown resolution now returns an error.
	_, err := resolvedText(b, gitdomain.ConflictResolution("unknown"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown resolution")
}

// ---------------------------------------------------------------------------
// replaceBlock
// ---------------------------------------------------------------------------

func TestReplaceBlock_Normal(
	t *testing.T,
) {
	content := "line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> b\nline7\n"
	b := &conflictBlock{startLine: 2, endLine: 6}
	result := replaceBlock(content, b, "resolved")
	assert.Contains(t, result, "resolved")
	assert.NotContains(t, result, "<<<<<<<")
}

// TestReplaceBlock_StartNegative exercises the start < 0 branch.
func TestReplaceBlock_StartNegative(
	t *testing.T,
) {
	content := "line1\nline2"
	b := &conflictBlock{startLine: 0, endLine: 1} // startLine=0 → start=-1
	result := replaceBlock(content, b, "x")
	assert.NotEmpty(t, result)
}

// TestReplaceBlock_EndBeyondLength exercises the end > len(lines) branch.
func TestReplaceBlock_EndBeyondLength(
	t *testing.T,
) {
	content := "line1\nline2"
	lines := []string{"line1", "line2"}
	b := &conflictBlock{startLine: 1, endLine: len(lines) + 10} // end > len
	result := replaceBlock(content, b, "replaced")
	assert.Equal(t, "replaced", result)
}

// ---------------------------------------------------------------------------
// hasMarkers
// ---------------------------------------------------------------------------

func TestHasMarkers_True(
	t *testing.T,
) {
	content := "line1\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> b\n"
	assert.True(t, hasMarkers(content))
}

func TestHasMarkers_False(
	t *testing.T,
) {
	content := "clean file\nno conflicts\n"
	assert.False(t, hasMarkers(content))
}

// ---------------------------------------------------------------------------
// conflictHunkID
// ---------------------------------------------------------------------------

// TestConflictHunkID_StableAcrossLineShift verifies that the content-based ID
// does not change when the line number changes (unlike the old position-based
// hunkID).
func TestConflictHunkID_StableAcrossLineShift(
	t *testing.T,
) {
	id1 := conflictHunkID("file.go", "ours content", "theirs content")
	id2 := conflictHunkID("file.go", "ours content", "theirs content")
	assert.Equal(t, id1, id2)
	assert.Len(t, id1, 12)
}

func TestConflictHunkID_DifferentContent(
	t *testing.T,
) {
	id1 := conflictHunkID("file.go", "ours A", "theirs A")
	id2 := conflictHunkID("file.go", "ours B", "theirs B")
	assert.NotEqual(t, id1, id2)
}
