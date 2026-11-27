package diff_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

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
