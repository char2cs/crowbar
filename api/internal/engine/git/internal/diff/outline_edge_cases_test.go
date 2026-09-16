package diff_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// TestOutline_NoTrailingNewlineInHunk pins the "\ No newline at end of file"
// marker line git emits for a file lacking a final newline: it belongs to
// neither side of the diff and must be consumed as a no-op (outline.go's
// `case '\\':`) rather than decrementing the hunk's remaining line counters,
// which would desynchronize the state machine from the header's declared
// shape.
func TestOutline_NoTrailingNewlineInHunk(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "notrail.txt", "line1\nline2\nline3")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "notrail.txt", "line1\nline2\nCHANGED")

	files, err := diff.Outline(context.Background(), repo, ref)

	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, []gitdomain.HunkShape{
		{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3},
	}, files[0].Hunks)
}

// TestOutline_BinaryFileWithNonASCIINameAndNoPlusLine pins diffGitPath's
// quoted-path branch: git C-quotes a "diff --git" line's paths whenever they
// carry a non-ASCII byte, and a binary file's entry has no "+++" line for
// newSidePath to prefer instead — diffGitPath is the ONLY source for its path,
// and must unquote it rather than returning the raw octal-escaped literal.
func TestOutline_BinaryFileWithNonASCIINameAndNoPlusLine(t *testing.T) {
	repo := initRepo(t)
	const name = "wärme.bin"
	writeFile(t, repo, name, "\x00\x01\x02")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, name, "\x09\x08\x07")

	files, err := diff.Outline(context.Background(), repo, ref)

	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, name, files[0].Path)
	assert.True(t, files[0].IsBinary)
}
