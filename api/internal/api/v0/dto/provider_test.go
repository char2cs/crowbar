package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

func TestProviderStateDTOFrom_WithPR(
	t *testing.T,
) {
	got := dto.ProviderStateDTOFrom(providertypes.ProviderState{
		Protected: true,
		PR: &providertypes.PRInfo{
			Number:       7,
			Status:       "open",
			URL:          "https://example.com/pr/7",
			Title:        "feat: thing",
			TargetBranch: "main",
		},
	})
	assert.True(t, got.Protected)
	require.NotNil(t, got.PR)
	assert.Equal(t, 7, got.PR.Number)
	assert.Equal(t, "open", got.PR.Status)
	assert.Equal(t, "https://example.com/pr/7", got.PR.URL)
	assert.Equal(t, "feat: thing", got.PR.Title)
	assert.Equal(t, "main", got.PR.TargetBranch)
}

func TestProviderStateDTOFrom_NoPR(
	t *testing.T,
) {
	got := dto.ProviderStateDTOFrom(providertypes.ProviderState{Protected: false})
	assert.False(t, got.Protected)
	assert.Nil(t, got.PR)
}

func TestProtectedBranchListEmptyNonNil(
	t *testing.T,
) {
	got := dto.ProtectedBranchList(nil)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestProtectedBranchList(
	t *testing.T,
) {
	got := dto.ProtectedBranchList([]string{"main", "release"})
	assert.Equal(t, []string{"main", "release"}, got)
}
