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

// TestChatWorktreeFrom_CarriesEveryFieldTheWorkspaceDTOCarries is the
// anti-drift test for spec §5's whole promise: ONE read of a chat has to answer
// everything the deleted workspace list answered, so every field a client used
// to read off WorkspaceDTO must survive the projection onto the chat.
//
// It asserts against a fully-populated source rather than a couple of sampled
// fields, because the failure this guards against is a field being ADDED to
// WorkspaceDTO later and silently not reaching the chat — which a sampled test
// would never see.
func TestChatWorktreeFrom_CarriesEveryFieldTheWorkspaceDTOCarries(t *testing.T) {
	source := dto.WorkspaceDTOFrom(
		domain.Workspace{
			ID:             "ws-1",
			RepoID:         "r1",
			ProjectID:      "p1",
			Branch:         "feature/x",
			ParentID:       "ws-0",
			ForkPointSha:   "abc123",
			Status:         domain.WorkspaceStatusNew,
			Working:        true,
			LastError:      "a push failed",
			IsDefault:      true,
			Added:          7,
			Deleted:        3,
			MergeStrategy:  gitdomain.MergeStrategySquash,
			PRUrl:          "https://example.invalid/pr/1",
			PRTitle:        "Do the thing",
			PRTargetBranch: "main",
			WorktreePath:   "/tmp/wt",
			HeldByPath:     "/tmp/held",
		},
		workspace.MergeEligibility{CanMergeLocally: true, ParentBranch: "main"},
		"chat-owner",
	)

	got := dto.ChatWorktreeFrom(source)

	require.NotNil(t, got)
	assert.Equal(t, source.Branch, got.Branch)
	assert.Equal(t, source.Status, got.Status, "the conflict-overlaid status, never the raw one")
	assert.Equal(t, source.LastError, got.LastError)
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
}

// TestChatWorktreeFrom_TakesTheConflictOverlaidStatus proves the projection
// goes through WorkspaceDTOFrom rather than reading domain.Workspace directly:
// a branch predicted to conflict with its parent is pr-conflicts on the wire,
// and a chat that reported the raw persisted status instead would disagree with
// the workspace describing the very same branch.
func TestChatWorktreeFrom_TakesTheConflictOverlaidStatus(t *testing.T) {
	source := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "ws-1", Status: domain.WorkspaceStatusNew},
		workspace.MergeEligibility{MergeConflicts: true},
		"chat-owner",
	)

	got := dto.ChatWorktreeFrom(source)

	require.NotNil(t, got)
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, got.Status)
	assert.True(t, got.MergeConflicts)
}

// TestAgentChatDTOFrom_ABubbleOmitsTheWorktreeEntirely pins the absent-is-
// meaningful encoding. A chat that owns no worktree does not have a branch
// named "" with nothing added and nothing deleted — it has no branch at all,
// and a zero object on the wire would have every conversation in the panel
// drawing an empty diff badge.
func TestAgentChatDTOFrom_ABubbleOmitsTheWorktreeEntirely(t *testing.T) {
	got := dto.AgentChatDTOFrom(domain.Chat{ID: "c1"}, dto.ChatRuntime{}, nil)

	assert.Nil(t, got.Worktree)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "worktree",
		"absent, not present-and-zero: the key must not appear at all")
}

// TestAgentChatDTOFrom_AWorktreeOwningChatCarriesItsGitState is the other half:
// the chat list is now the ONE read a client makes to draw a branch row, so a
// chat that owns a worktree has to carry the branch on its own object.
func TestAgentChatDTOFrom_AWorktreeOwningChatCarriesItsGitState(t *testing.T) {
	worktree := &dto.ChatWorktreeDTO{Branch: "feature/x", Added: 2, OwningChatID: "c1"}

	got := dto.AgentChatDTOFrom(
		domain.Chat{ID: "c1", WorkspaceID: "ws-1"}, dto.ChatRuntime{}, worktree)

	require.NotNil(t, got.Worktree)
	assert.Equal(t, "feature/x", got.Worktree.Branch)
	assert.Equal(t, "ws-1", got.WorkspaceID)
}

// TestAgentChatDTOList_ResolvesEachRowsWorktreeThroughTheClosure mirrors
// WorkspaceDTOList's own eligFn/owningChatIDFn contract: the CALLER supplies a
// closure built over the repo-wide reads it has already taken once, so the
// enrichment costs one repo read for a whole list rather than one per row.
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

// TestAgentChatDTOList_ANilClosureLeavesEveryWorktreeAbsent keeps the
// folder-only surfaces honest: they mount these handlers without the workspace
// reads wired, and a folder owns no worktree by construction, so "no closure"
// must mean "no worktree" rather than a panic.
func TestAgentChatDTOList_ANilClosureLeavesEveryWorktreeAbsent(t *testing.T) {
	got := dto.AgentChatDTOList(
		[]domain.Chat{{ID: "f1", Type: domain.ChatTypeFolder}}, nil, nil)

	require.Len(t, got, 1)
	assert.Nil(t, got[0].Worktree)
}

// TestAgentChatEvent_WorktreeStateFrameCarriesTheGitState pins the live half of
// §5. The chat feed has no snapshot, so a client that had to refetch on every
// git change would repaint a round trip after the diff counts moved — the same
// reasoning that puts Working and TerminalWait on the frame.
func TestAgentChatEvent_WorktreeStateFrameCarriesTheGitState(t *testing.T) {
	frame := dto.AgentChatEvent{
		ChatID:      "c1",
		WorkspaceID: "ws-1",
		RepoID:      "r1",
		Kind:        dto.AgentChatKindWorktreeState,
		Worktree:    &dto.ChatWorktreeDTO{Branch: "feature/x", Added: 4, OwningChatID: "c1"},
	}

	raw, err := json.Marshal(frame)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"kind":"worktree_state"`)
	assert.Contains(t, string(raw), `"branch":"feature/x"`)
	assert.Contains(t, string(raw), `"owningChatId":"c1"`)
}

// A frame about anything BUT the worktree carries no worktree key at all, so a
// client merging frames into its cache never blanks a branch row on a
// turn_started it happened to receive first.
func TestAgentChatEvent_ANonWorktreeFrameOmitsIt(t *testing.T) {
	raw, err := json.Marshal(dto.AgentChatEvent{ChatID: "c1", Kind: "turn_started"})

	require.NoError(t, err)
	assert.NotContains(t, string(raw), "worktree")
}
