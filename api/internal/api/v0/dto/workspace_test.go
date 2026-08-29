package dto_test

import (
	"encoding/json"
	"testing"
	"time"

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

// TestWorkspaceDTOFrom_EffectiveStatus covers the read-time status overlay:
// a predicted conflict folds into pr-conflicts, but a terminal/locked base
// status takes precedence and a non-conflicting read keeps the base status.
func TestWorkspaceDTOFrom_EffectiveStatus(t *testing.T) {
	cases := []struct {
		name     string
		base     domain.WorkspaceStatus
		conflict bool
		want     domain.WorkspaceStatus
	}{
		{"conflict folds new -> pr-conflicts", domain.WorkspaceStatusNew, true, domain.WorkspaceStatusPRConflicts},
		{"conflict folds pr-open -> pr-conflicts", domain.WorkspaceStatusPROpen, true, domain.WorkspaceStatusPRConflicts},
		{"locked base wins over conflict", domain.WorkspaceStatusLocked, true, domain.WorkspaceStatusLocked},
		{"deleted base wins over conflict", domain.WorkspaceStatusDeleted, true, domain.WorkspaceStatusDeleted},
		{"no conflict keeps base", domain.WorkspaceStatusPROpen, false, domain.WorkspaceStatusPROpen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dto.WorkspaceDTOFrom(
				domain.Workspace{ID: "w", Status: tc.base},
				workspace.MergeEligibility{MergeConflicts: tc.conflict},
			)
			assert.Equal(t, tc.want, got.Status)
			assert.Equal(t, tc.conflict, got.MergeConflicts)
		})
	}
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
	assert.Equal(t, "/wt", got.LocalPath)
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
		ID:           "w1",
		Working:      true,
		LastError:    "oops",
		WorktreePath: "/wt",
	}, workspace.MergeEligibility{}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"working", "lastError", "canMergeLocally", "localPath"} {
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

// The list is ordered by the converter itself, which is what makes the REST list
// and the WS snapshot — the two callers — incapable of disagreeing about
// ordering. Placement no longer lives on this resource at all (it is the row's
// own chat row in the unified sidebar tree), so creation time is the whole
// ordering the list has left to offer on its own.
func TestWorkspaceDTOList_OrdersByCreatedAt(
	t *testing.T,
) {
	got := dto.WorkspaceDTOList([]domain.Workspace{
		{ID: "c", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "b", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "a", CreatedAt: time.Unix(2, 0).UTC()},
	}, noElig)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"c", "a", "b"}, []string{got[0].ID, got[1].ID, got[2].ID})
}

// TestWorkspaceDTOFrom_NeverLeaksAFolderIntoTheForkLineage pins that ParentID
// stays the fork lineage alone — three git paths resolve it back to a
// workspace — now that FolderID/Order are gone from the resource entirely.
func TestWorkspaceDTOFrom_NeverLeaksAFolderIntoTheForkLineage(
	t *testing.T,
) {
	got := dto.WorkspaceDTOFrom(domain.Workspace{
		ID: "w1", ParentID: "",
	}, workspace.MergeEligibility{})

	assert.Empty(t, got.ParentID, "a folder id must never leak into the fork lineage")
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

func TestWorkspaceDTOFrom_MapsIsDefault(t *testing.T) {
	got := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", IsDefault: true},
		workspace.MergeEligibility{},
	)
	assert.True(t, got.IsDefault)

	got2 := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "w2", RepoID: "r1", ProjectID: "p1"},
		workspace.MergeEligibility{},
	)
	assert.False(t, got2.IsDefault)
}

// TestWorkspaceDTOFrom_MapsHeldByPath proves the placeholder holder path
// (domain.Workspace.HeldByPath) is carried onto the wire DTO so the FE can
// reconstruct the placeholder reason from it (spec §4/B3).
func TestWorkspaceDTOFrom_MapsHeldByPath(t *testing.T) {
	got := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "w1", HeldByPath: "/Users/me/proj"},
		workspace.MergeEligibility{},
	)
	assert.Equal(t, "/Users/me/proj", got.HeldByPath)
}
