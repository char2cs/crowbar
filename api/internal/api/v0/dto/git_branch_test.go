package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestBranchDTOFromWithDate(
	t *testing.T,
) {
	ahead := 3
	behind := 1
	when := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	branch := gitdomain.Branch{
		Name:           "main",
		IsCurrent:      true,
		IsRemote:       false,
		Ahead:          &ahead,
		Behind:         &behind,
		LastCommitDate: &when,
	}

	got := dto.BranchDTOFrom(branch)

	assert.Equal(t, "main", got.Name)
	assert.True(t, got.IsCurrent)
	require.NotNil(t, got.Ahead)
	assert.Equal(t, 3, *got.Ahead)
	require.NotNil(t, got.Behind)
	assert.Equal(t, 1, *got.Behind)
	require.NotNil(t, got.LastCommitDate)
	assert.Equal(t, "2026-06-06T12:00:00Z", *got.LastCommitDate)
}

func TestBranchDTOFromNilDate(
	t *testing.T,
) {
	got := dto.BranchDTOFrom(gitdomain.Branch{Name: "dev"})

	assert.Equal(t, "dev", got.Name)
	assert.Nil(t, got.LastCommitDate)
}

func TestBranchDTOListEmptyIsNonNil(
	t *testing.T,
) {
	got := dto.BranchDTOList(nil)

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestBranchDTOListConverts(
	t *testing.T,
) {
	got := dto.BranchDTOList([]gitdomain.Branch{
		{Name: "a"},
		{Name: "b"},
	})

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Name)
	assert.Equal(t, "b", got[1].Name)
}
