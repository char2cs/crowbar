package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestBlameEntryDTOFromRendersRFC3339(
	t *testing.T,
) {
	entry := gitdomain.BlameEntry{
		LineNumber:    7,
		CommitHash:    "abc123",
		Author:        "Ada",
		Email:         "ada@x.io",
		Date:          time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		CommitMessage: "fix",
	}

	got := dto.BlameEntryDTOFrom(entry)

	assert.Equal(t, 7, got.LineNumber)
	assert.Equal(t, "abc123", got.CommitHash)
	assert.Equal(t, "Ada", got.Author)
	assert.Equal(t, "ada@x.io", got.Email)
	assert.Equal(t, "2026-06-06T12:00:00Z", got.Date)
	assert.Equal(t, "fix", got.CommitMessage)
}

func TestBlameEntryDTOListEmptyIsNonNil(
	t *testing.T,
) {
	got := dto.BlameEntryDTOList(nil)

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestBlameEntryDTOListConverts(
	t *testing.T,
) {
	entries := []gitdomain.BlameEntry{
		{LineNumber: 1, CommitHash: "a"},
		{LineNumber: 2, CommitHash: "b"},
	}

	got := dto.BlameEntryDTOList(entries)

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].CommitHash)
	assert.Equal(t, "b", got[1].CommitHash)
}
