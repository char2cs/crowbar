package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestGitStatusDTOFromConverts(
	t *testing.T,
) {
	status := gitdomain.GitStatus{
		Branch: "main",
		Ahead:  1,
		Behind: 2,
		Files: []gitdomain.GitFile{
			{Path: "a.go", Status: gitdomain.GitFileStatusModified, Staged: true},
		},
	}

	got := dto.GitStatusDTOFrom(status)

	assert.Equal(t, "main", got.Branch)
	assert.Equal(t, 1, got.Ahead)
	assert.Equal(t, 2, got.Behind)
	require.Len(t, got.Files, 1)
	assert.Equal(t, "a.go", got.Files[0].Path)
	assert.Equal(t, "modified", got.Files[0].Status)
	assert.True(t, got.Files[0].Staged)
}

func TestGitStatusDTOFromEmptyFilesIsNonNil(
	t *testing.T,
) {
	got := dto.GitStatusDTOFrom(gitdomain.GitStatus{})

	require.NotNil(t, got.Files)
	assert.Empty(t, got.Files)
}
