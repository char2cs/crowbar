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

func TestRepoDTOFrom_ProxyURLHierarchical(
	t *testing.T,
) {
	got := dto.RepoDTOFrom(domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		AvatarHasIcon: true,
	})
	assert.Equal(t, "/v0/projects/p1/repos/r1/icon?v=0", got.AvatarURL)
	assert.Empty(t, got.AvatarEmoji)
}

// TestRepoDTOFrom_ProxyURLCarriesAvatarVersion pins the cache-busting query
// param: uploads change the icon bytes behind a stable path, so the version
// must be part of the URL for clients to refetch the image.
func TestRepoDTOFrom_ProxyURLCarriesAvatarVersion(
	t *testing.T,
) {
	got := dto.RepoDTOFrom(domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		AvatarHasIcon: true,
		AvatarVersion: 3,
	})
	assert.Equal(t, "/v0/projects/p1/repos/r1/icon?v=3", got.AvatarURL)
}

func TestRepoDTOFrom_EmojiPassthrough(
	t *testing.T,
) {
	got := dto.RepoDTOFrom(domain.Repository{
		ID:          "r1",
		ProjectID:   "p1",
		AvatarEmoji: "🦊",
	})
	assert.Equal(t, "🦊", got.AvatarEmoji)
	assert.Empty(t, got.AvatarURL, "emoji-only repo has no proxy URL")
}

func TestRepoDTOFrom_NoIconEmptyAvatarURL(
	t *testing.T,
) {
	got := dto.RepoDTOFrom(domain.Repository{
		ID:        "r1",
		ProjectID: "p1",
	})
	assert.Empty(t, got.AvatarURL)
	assert.Empty(t, got.AvatarEmoji)
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

// The list is ordered by the converter itself, which is what makes the REST list
// and the WS snapshot — the two callers — incapable of disagreeing about the
// sidebar order.
func TestRepoDTOList_OrdersByIndexThenID(
	t *testing.T,
) {
	got := dto.RepoDTOList([]domain.Repository{
		{ID: "c", Order: 2},
		{ID: "b"},
		{ID: "a"},
	})
	require.Len(t, got, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{got[0].ID, got[1].ID, got[2].ID})
	assert.Equal(t, 2, got[2].Order, "the order reaches the wire")
}
