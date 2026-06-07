package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestConflictHunkDTOFrom(
	t *testing.T,
) {
	hunk := gitdomain.ConflictHunk{
		ID:              "h1",
		StartLine:       2,
		EndLine:         5,
		Ours:            "o",
		Theirs:          "t",
		Base:            "b",
		Resolution:      gitdomain.ConflictResolutionBoth,
		ResolvedContent: "rc",
	}
	got := dto.ConflictHunkDTOFrom(hunk)

	assert.Equal(t, "h1", got.ID)
	assert.Equal(t, 2, got.StartLine)
	assert.Equal(t, 5, got.EndLine)
	assert.Equal(t, "o", got.Ours)
	assert.Equal(t, "t", got.Theirs)
	assert.Equal(t, "b", got.Base)
	assert.Equal(t, "both", got.Resolution)
	assert.Equal(t, "rc", got.ResolvedContent)
}

func TestConflictHunkDTOListEmptyNonNil(
	t *testing.T,
) {
	got := dto.ConflictHunkDTOList(nil)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestConflictHunkDTOList(
	t *testing.T,
) {
	got := dto.ConflictHunkDTOList([]gitdomain.ConflictHunk{
		{ID: "h1"},
		{ID: "h2"},
	})
	assert.Len(t, got, 2)
	assert.Equal(t, "h1", got[0].ID)
	assert.Equal(t, "h2", got[1].ID)
}

func TestConflictFileDTOFromEmptyHunks(
	t *testing.T,
) {
	got := dto.ConflictFileDTOFrom("a.txt", nil)
	assert.Equal(t, "a.txt", got.Path)
	assert.NotNil(t, got.Hunks)
	assert.Empty(t, got.Hunks)
}

func TestConflictFileDTOFrom(
	t *testing.T,
) {
	got := dto.ConflictFileDTOFrom("a.txt", []gitdomain.ConflictHunk{{ID: "h1"}})
	assert.Equal(t, "a.txt", got.Path)
	assert.Len(t, got.Hunks, 1)
	assert.Equal(t, "h1", got.Hunks[0].ID)
}
