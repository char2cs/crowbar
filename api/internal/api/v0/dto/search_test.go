package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestSearchResponseToDTOMapsFields(
	t *testing.T,
) {
	got := dto.SearchResponseToDTO(enginesearch.SearchResponse{
		Results: []enginesearch.SearchResult{
			{
				FilePath:   "main.go",
				LineNumber: 7,
				LineText:   "package main",
				MatchStart: 8,
				MatchEnd:   12,
			},
		},
		Truncated: true,
	})

	require.Len(t, got.Results, 1)
	assert.Equal(t, "main.go", got.Results[0].FilePath)
	assert.Equal(t, 7, got.Results[0].LineNumber)
	assert.Equal(t, "package main", got.Results[0].LineText)
	assert.Equal(t, 8, got.Results[0].MatchStart)
	assert.Equal(t, 12, got.Results[0].MatchEnd)
	assert.True(t, got.Truncated)
}

func TestSearchResponseToDTOEmptyResultsIsNonNil(
	t *testing.T,
) {
	got := dto.SearchResponseToDTO(enginesearch.SearchResponse{})

	require.NotNil(t, got.Results)
	assert.Empty(t, got.Results)
	assert.False(t, got.Truncated)
}
