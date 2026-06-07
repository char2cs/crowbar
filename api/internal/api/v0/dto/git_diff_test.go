package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestFileDiffDTOListEmptyIsNonNil(
	t *testing.T,
) {
	got := dto.FileDiffDTOList(nil)

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestFileDiffDTOListNormalizesNestedSlices(
	t *testing.T,
) {
	got := dto.FileDiffDTOList([]gitdomain.FileDiff{
		{FilePath: "a.go"},
	})

	require.Len(t, got, 1)
	assert.Equal(t, "a.go", got[0].FilePath)
	require.NotNil(t, got[0].Lines)
	assert.Empty(t, got[0].Lines)
	require.NotNil(t, got[0].Hunks)
	assert.Empty(t, got[0].Hunks)
}

func TestFileDiffDTOListPreservesHunkID(
	t *testing.T,
) {
	got := dto.FileDiffDTOList([]gitdomain.FileDiff{
		{
			FilePath: "a.go",
			Lines:    []gitdomain.DiffLine{{Content: "x", HunkID: "h1"}},
			Hunks:    []gitdomain.Hunk{{HunkID: "h1", StartLine: 0, EndLine: 0}},
		},
	})

	require.Len(t, got, 1)
	require.Len(t, got[0].Hunks, 1)
	assert.Equal(t, "h1", got[0].Hunks[0].HunkID)
	require.Len(t, got[0].Lines, 1)
	assert.Equal(t, "h1", got[0].Lines[0].HunkID)
}

func TestMultiFileDiffDTOFromConverts(
	t *testing.T,
) {
	got := dto.MultiFileDiffDTOFrom(gitdomain.MultiFileDiff{
		CommitHash: "deadbeef",
		Files:      []gitdomain.FileDiff{{FilePath: "a.go"}},
		TotalFiles: 1,
	})

	assert.Equal(t, "deadbeef", got.CommitHash)
	assert.Equal(t, 1, got.TotalFiles)
	require.Len(t, got.Files, 1)
	require.NotNil(t, got.Files[0].Lines)
	require.NotNil(t, got.Files[0].Hunks)
}

func TestMultiFileDiffDTOFromEmptyFilesIsNonNil(
	t *testing.T,
) {
	got := dto.MultiFileDiffDTOFrom(gitdomain.MultiFileDiff{})

	require.NotNil(t, got.Files)
	assert.Empty(t, got.Files)
}
