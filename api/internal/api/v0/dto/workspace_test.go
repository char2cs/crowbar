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

func noElig(
	_ domain.Workspace,
) workspace.MergeEligibility {
	return workspace.MergeEligibility{}
}

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
		Added:          3,
		Deleted:        2,
		MergeStrategy:  gitdomain.MergeStrategySquash,
		PRUrl:          "http://pr",
		PRTitle:        "title",
		PRTargetBranch: "main",
		Working:        true,
		LastError:      "boom",
	}, workspace.MergeEligibility{})
	assert.Equal(t, "w1", got.ID)
	assert.Equal(t, "r1", got.RepoID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "feat", got.Branch)
	assert.Equal(t, "w0", got.ParentID)
	assert.Equal(t, "deadbeef", got.ForkPointSha)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status)
	assert.Equal(t, 3, got.Added)
	assert.Equal(t, 2, got.Deleted)
	assert.Equal(t, gitdomain.MergeStrategySquash, got.MergeStrategy)
	assert.Equal(t, "http://pr", got.PRUrl)
	assert.Equal(t, "title", got.PRTitle)
	assert.Equal(t, "main", got.PRTargetBranch)
	assert.True(t, got.Working)
	assert.Equal(t, "boom", got.LastError)
	// Eligibility is supplied by the caller; an empty overlay maps to zero
	// values.
	assert.False(t, got.CanMergeLocally)
	assert.Equal(t, "", got.ParentBranch)
}

// TestWorkspaceDTOFrom_MapsEligibility pins that the resolved merge-eligibility
// overlay the caller computes is mapped onto the wire DTO (spec §10).
func TestWorkspaceDTOFrom_MapsEligibility(
	t *testing.T,
) {
	got := dto.WorkspaceDTOFrom(domain.Workspace{ID: "w1", ParentID: "w0"}, workspace.MergeEligibility{
		CanMergeLocally: true,
		ParentBranch:    "main",
	})
	assert.True(t, got.CanMergeLocally)
	assert.Equal(t, "main", got.ParentBranch)
}

// TestWorkspaceDTO_WireFields pins the final wire contract: the field set and
// json tags. The eligibility/error overlay fields (working, lastError,
// canMergeLocally, parentBranch) are present; the retired legacy fields
// (locked, hasConflicts, agentRunning, pendingMerge, worktreePath) are absent.
func TestWorkspaceDTO_WireFields(
	t *testing.T,
) {
	raw, err := json.Marshal(dto.WorkspaceDTOFrom(domain.Workspace{
		ID:        "w1",
		Working:   true,
		LastError: "oops",
	}, workspace.MergeEligibility{}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"working", "lastError", "canMergeLocally"} {
		_, present := decoded[key]
		assert.Truef(t, present, "expected wire key %q to be present", key)
	}
	assert.Equal(t, true, decoded["working"])
	assert.Equal(t, "oops", decoded["lastError"])
	assert.Equal(t, false, decoded["canMergeLocally"])

	for _, key := range []string{
		"locked",
		"hasConflicts",
		"agentRunning",
		"pendingMerge",
		"worktreePath",
		"parentBranch", // omitempty: absent when empty
	} {
		_, present := decoded[key]
		assert.Falsef(t, present, "expected wire key %q to be absent", key)
	}
}

// TestWorkspaceDTO_ParentBranchOmitEmpty pins that parentBranch is emitted when
// non-empty and omitted when empty.
func TestWorkspaceDTO_ParentBranchOmitEmpty(
	t *testing.T,
) {
	dtoVal := dto.WorkspaceDTOFrom(domain.Workspace{ID: "w1"}, workspace.MergeEligibility{})
	dtoVal.CanMergeLocally = true
	dtoVal.ParentBranch = "main"

	raw, err := json.Marshal(dtoVal)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, true, decoded["canMergeLocally"])
	assert.Equal(t, "main", decoded["parentBranch"])
}

func TestWorkspaceDTOListEmptyNonNil(
	t *testing.T,
) {
	got := dto.WorkspaceDTOList(nil, noElig)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestWorkspaceDTOList(
	t *testing.T,
) {
	got := dto.WorkspaceDTOList([]domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	}, noElig)
	require.Len(t, got, 2)
	assert.Equal(t, "w1", got[0].ID)
	assert.Equal(t, "w2", got[1].ID)
}

// TestWorkspaceDTOList_AppliesEligFn pins that the per-row eligibility resolver
// is invoked and its result mapped onto each DTO (spec §10).
func TestWorkspaceDTOList_AppliesEligFn(
	t *testing.T,
) {
	eligFn := func(w domain.Workspace) workspace.MergeEligibility {
		if w.ID == "w1" {
			return workspace.MergeEligibility{CanMergeLocally: true, ParentBranch: "main"}
		}
		return workspace.MergeEligibility{}
	}
	got := dto.WorkspaceDTOList([]domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	}, eligFn)
	require.Len(t, got, 2)
	assert.True(t, got[0].CanMergeLocally)
	assert.Equal(t, "main", got[0].ParentBranch)
	assert.False(t, got[1].CanMergeLocally)
	assert.Equal(t, "", got[1].ParentBranch)
}
