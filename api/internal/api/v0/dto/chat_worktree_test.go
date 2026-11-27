package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestChatWorktreeFrom_CarriesEveryFieldTheWorkspaceDTOCarries(t *testing.T) {
	source := dto.WorkspaceDTOFrom(
		ctx,
		domain.Workspace{
			ID:             "ws-1",
			RepoID:         "r1",
			ProjectID:      "p1",
			Branch:         "feature/x",
			ParentID:       "ws-0",
			ForkPointSha:   "abc123",
			Status:         domain.WorkspaceStatusNew,
			Working:        true,
			IsDefault:      true,
			Added:          7,
			Deleted:        3,
			MergeStrategy:  gitdomain.MergeStrategySquash,
			PRUrl:          "https://example.invalid/pr/1",
			PRTitle:        "Do the thing",
			PRTargetBranch: "main",
			WorktreePath:   "/tmp/wt",
			HeldByPath:     "/tmp/held",
			Provisioning:   domain.WorkspaceShared,
		},
		workspace.MergeEligibility{CanMergeLocally: true, ParentBranch: "main"},
		"chat-owner",
		fakePlacement{"ws-1": {folderID: "docs", order: 6}},
	)

	got := dto.ChatWorktreeFrom(source)

	require.NotNil(t, got)
	assert.Equal(t, source.Branch, got.Branch)
	assert.Equal(t, source.Status, got.Status, "the conflict-overlaid status, never the raw one")
	assert.Equal(t, source.Working, got.Working)
	assert.Equal(t, source.IsDefault, got.IsDefault)
	assert.Equal(t, source.Added, got.Added)
	assert.Equal(t, source.Deleted, got.Deleted)
	assert.Equal(t, source.MergeStrategy, got.MergeStrategy)
	assert.Equal(t, source.CanMergeLocally, got.CanMergeLocally)
	assert.Equal(t, source.MergeConflicts, got.MergeConflicts)
	assert.Equal(t, source.ParentBranch, got.ParentBranch)
	assert.Equal(t, source.PRUrl, got.PRUrl)
	assert.Equal(t, source.PRTitle, got.PRTitle)
	assert.Equal(t, source.PRTargetBranch, got.PRTargetBranch)
	assert.Equal(t, source.LocalPath, got.LocalPath)
	assert.Equal(t, source.HeldByPath, got.HeldByPath)
	assert.Equal(t, source.ForkPointSha, got.ForkPointSha)
	assert.Equal(t, source.ParentID, got.ParentID)
	assert.Equal(t, source.OwningChatID, got.OwningChatID,
		"which chat owns the worktree is the daemon's answer, never the client's to derive")
	assert.Equal(t, source.FolderID, got.FolderID)
	assert.Equal(t, source.Order, got.Order)
}

func TestChatWorktreeFrom_TakesTheConflictOverlaidStatus(t *testing.T) {
	source := dto.WorkspaceDTOFrom(
		ctx,
		domain.Workspace{ID: "ws-1", Status: domain.WorkspaceStatusNew},
		workspace.MergeEligibility{MergeConflicts: true},
		"chat-owner",
		nil,
	)

	got := dto.ChatWorktreeFrom(source)

	require.NotNil(t, got)
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, got.Status)
	assert.True(t, got.MergeConflicts)
}

func TestAgentChatDTOFrom_ABubbleOmitsTheWorktreeEntirely(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{}, nil)

	assert.Nil(t, got.Worktree)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "worktree",
		"absent, not present-and-zero: the key must not appear at all")
}

func TestAgentChatDTOFrom_AWorktreeOwningChatCarriesItsGitState(t *testing.T) {
	worktree := &dto.ChatWorktreeDTO{Branch: "feature/x", Added: 2, OwningChatID: "c1"}

	got := dto.AgentChatDTOFrom(
		domain.Chat{ID: "c1", WorkspaceID: "ws-1"}, dto.ChatRuntime{}, worktree)

	require.NotNil(t, got.Worktree)
	assert.Equal(t, "feature/x", got.Worktree.Branch)
	assert.Equal(t, "ws-1", got.WorkspaceID)
}

func TestAgentChatDTOList_ResolvesEachRowsWorktreeThroughTheClosure(t *testing.T) {
	rows := []domain.Chat{
		{ID: "owner", WorkspaceID: "ws-1"},
		{ID: "bubble"},
	}

	got := dto.AgentChatDTOList(rows, nil, func(c domain.Chat) *dto.ChatWorktreeDTO {
		if c.WorkspaceID == "" {
			return nil
		}
		return &dto.ChatWorktreeDTO{Branch: "feature/x", OwningChatID: c.ID}
	})

	require.Len(t, got, 2)
	require.NotNil(t, got[0].Worktree)
	assert.Equal(t, "feature/x", got[0].Worktree.Branch)
	assert.Nil(t, got[1].Worktree, "a bubble owns none")
}

func TestAgentChatDTOList_ANilClosureLeavesEveryWorktreeAbsent(t *testing.T) {
	got := dto.AgentChatDTOList(
		[]domain.Chat{{ID: "f1", Type: domain.ChatTypeFolder}}, nil, nil)

	require.Len(t, got, 1)
	assert.Nil(t, got[0].Worktree)
}
