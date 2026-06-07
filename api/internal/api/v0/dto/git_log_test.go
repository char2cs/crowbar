package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestCommitDTOFromRendersRFC3339(
	t *testing.T,
) {
	commit := gitdomain.Commit{
		Hash:        "abc",
		ShortHash:   "ab",
		Message:     "init",
		Description: "body",
		Author:      "Ada",
		Email:       "ada@x.io",
		Date:        time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}

	got := dto.CommitDTOFrom(commit)

	assert.Equal(t, "abc", got.Hash)
	assert.Equal(t, "ab", got.ShortHash)
	assert.Equal(t, "init", got.Message)
	assert.Equal(t, "body", got.Description)
	assert.Equal(t, "Ada", got.Author)
	assert.Equal(t, "ada@x.io", got.Email)
	assert.Equal(t, "2026-06-06T12:00:00Z", got.Date)
}

func TestCommitDTOListEmptyIsNonNil(
	t *testing.T,
) {
	got := dto.CommitDTOList(nil)

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestCommitDTOListConverts(
	t *testing.T,
) {
	got := dto.CommitDTOList([]gitdomain.Commit{
		{Hash: "a"},
		{Hash: "b"},
	})

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Hash)
	assert.Equal(t, "b", got[1].Hash)
}
