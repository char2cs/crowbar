package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRepoDTOFrom(
	t *testing.T,
) {
	got := dto.RepoDTOFrom(domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Name:          "alpha",
		Path:          "/a",
		DefaultBranch: "main",
		AvatarLabel:   "AL",
		AvatarColor:   "#fff",
	})
	assert.Equal(t, "r1", got.ID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "alpha", got.Name)
	assert.Equal(t, "/a", got.Path)
	assert.Equal(t, "main", got.DefaultBranch)
	assert.Equal(t, "AL", got.AvatarLabel)
	assert.Equal(t, "#fff", got.AvatarColor)
}

func TestRepoDTOListEmptyNonNil(
	t *testing.T,
) {
	got := dto.RepoDTOList(nil)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestRepoDTOList(
	t *testing.T,
) {
	got := dto.RepoDTOList([]domain.Repository{
		{ID: "r1"},
		{ID: "r2"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "r1", got[0].ID)
	assert.Equal(t, "r2", got[1].ID)
}
