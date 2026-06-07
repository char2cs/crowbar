package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestWorkspaceDTOFrom(
	t *testing.T,
) {
	got := dto.WorkspaceDTOFrom(domain.Workspace{
		ID:             "w1",
		RepoID:         "r1",
		ProjectID:      "p1",
		Branch:         "feat",
		WorktreePath:   "/wt",
		ForkPointSha:   "deadbeef",
		ParentID:       "w0",
		Status:         domain.WorkspaceStatusNew,
		Locked:         true,
		HasConflicts:   true,
		Added:          3,
		Deleted:        2,
		MergeStrategy:  gitdomain.MergeStrategySquash,
		PRUrl:          "http://pr",
		PRTitle:        "title",
		PRTargetBranch: "main",
		AgentRunning:   true,
	})
	assert.Equal(t, "w1", got.ID)
	assert.Equal(t, "r1", got.RepoID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "feat", got.Branch)
	assert.Equal(t, "w0", got.ParentID)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status)
	assert.True(t, got.Locked)
	assert.True(t, got.HasConflicts)
	assert.Equal(t, 3, got.Added)
	assert.Equal(t, 2, got.Deleted)
	assert.Equal(t, gitdomain.MergeStrategySquash, got.MergeStrategy)
	assert.Equal(t, "http://pr", got.PRUrl)
	assert.Equal(t, "title", got.PRTitle)
	assert.Equal(t, "main", got.PRTargetBranch)
	assert.True(t, got.AgentRunning)
}

func TestWorkspaceDTOListEmptyNonNil(
	t *testing.T,
) {
	got := dto.WorkspaceDTOList(nil)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestWorkspaceDTOList(
	t *testing.T,
) {
	got := dto.WorkspaceDTOList([]domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "w1", got[0].ID)
	assert.Equal(t, "w2", got[1].ID)
}
