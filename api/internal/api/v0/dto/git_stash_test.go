package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestStashDTOFromRendersRFC3339(
	t *testing.T,
) {
	stash := gitdomain.Stash{
		ID:           "stash@{0}",
		Message:      "wip",
		Date:         time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		FilesChanged: 2,
	}

	got := dto.StashDTOFrom(stash)

	assert.Equal(t, "stash@{0}", got.ID)
	assert.Equal(t, "wip", got.Message)
	assert.Equal(t, "2026-06-06T12:00:00Z", got.Date)
	assert.Equal(t, 2, got.FilesChanged)
}

func TestStashDTOListEmptyIsNonNil(
	t *testing.T,
) {
	got := dto.StashDTOList(nil)

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestStashDTOListConverts(
	t *testing.T,
) {
	got := dto.StashDTOList([]gitdomain.Stash{
		{ID: "a"},
		{ID: "b"},
	})

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "b", got[1].ID)
}
