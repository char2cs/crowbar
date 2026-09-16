package dto_test

import (
	"context"
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

var ctx = context.Background()

// placementRow is one workspace's fake sidebar placement.
type placementRow struct {
	folderID string
	order    int
}

// fakePlacement fakes dto.WorkspacePlacementReader, keyed by workspace id. A
// workspace never Set here answers "" / 0, the same degrade a nil reader
// gives.
type fakePlacement map[string]placementRow

func (f fakePlacement) Placement(_ context.Context, workspaceID string) (string, int) {
	row := f[workspaceID]
	return row.folderID, row.order
}

func noElig(
	_ domain.Workspace,
) workspace.MergeEligibility {
	return workspace.MergeEligibility{}
}

func noOwningChatID(
	_ domain.Workspace,
) string {
	return ""
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
				ctx,
				domain.Workspace{ID: "w", Status: tc.base},
				workspace.MergeEligibility{MergeConflicts: tc.conflict},
				"",
				nil,
			)
			assert.Equal(t, tc.want, got.Status)
			assert.Equal(t, tc.conflict, got.MergeConflicts)
		})
	}
}

func TestWorkspaceDTOFrom(
	t *testing.T,
) {
	got := dto.WorkspaceDTOFrom(ctx, domain.Workspace{
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
	}, workspace.MergeEligibility{}, "chat-1",
		fakePlacement{"w1": {folderID: "f1", order: 4}})
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
	assert.Equal(t, "chat-1", got.OwningChatID)
	assert.Equal(t, "f1", got.FolderID)
	assert.Equal(t, 4, got.Order)
}

// TestWorkspaceDTOFrom_MapsEligibility pins that the resolved merge-eligibility
// overlay the caller computes is mapped onto the wire DTO (spec §10).
func TestWorkspaceDTOFrom_MapsEligibility(
	t *testing.T,
) {
	got := dto.WorkspaceDTOFrom(ctx, domain.Workspace{ID: "w1", ParentID: "w0"}, workspace.MergeEligibility{
		CanMergeLocally: true,
		ParentBranch:    "main",
	}, "", nil)
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
	raw, err := json.Marshal(dto.WorkspaceDTOFrom(ctx, domain.Workspace{
		ID:           "w1",
		Working:      true,
		LastError:    "oops",
		WorktreePath: "/wt",
	}, workspace.MergeEligibility{}, "chat-1", fakePlacement{"w1": {folderID: "f1", order: 2}}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{
		"working", "lastError", "canMergeLocally", "localPath", "owningChatId", "folderId", "order",
	} {
		_, present := decoded[key]
		assert.Truef(t, present, "expected wire key %q to be present", key)
	}
	assert.Equal(t, true, decoded["working"])
	assert.Equal(t, "oops", decoded["lastError"])
	assert.Equal(t, false, decoded["canMergeLocally"])
	assert.Equal(t, "chat-1", decoded["owningChatId"])
	assert.Equal(t, "f1", decoded["folderId"])
	assert.Equal(t, float64(2), decoded["order"])

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

// TestWorkspaceDTO_OwningChatIDNeverOmitted pins that owningChatId, unlike
// parentBranch, stays on the wire even when it is the empty string: a client
// must be able to tell "this workspace genuinely has no owning chat yet" (a
// bug post-Task-3) apart from "the field was never sent".
func TestWorkspaceDTO_OwningChatIDNeverOmitted(t *testing.T) {
	raw, err := json.Marshal(dto.WorkspaceDTOFrom(ctx, domain.Workspace{ID: "w1"}, workspace.MergeEligibility{}, "", nil))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	value, present := decoded["owningChatId"]
	assert.True(t, present, "owningChatId must be present even when empty")
	assert.Equal(t, "", value)
}

// TestWorkspaceDTO_FolderIDAndOrderNeverOmitted mirrors
// TestWorkspaceDTO_OwningChatIDNeverOmitted for the two newer placement
// fields: a client must be able to tell "this row sits at the repo root,
// first slot" (the real zero value) apart from "the field was never sent"
// (an old daemon), so neither carries omitempty.
func TestWorkspaceDTO_FolderIDAndOrderNeverOmitted(t *testing.T) {
	raw, err := json.Marshal(dto.WorkspaceDTOFrom(ctx, domain.Workspace{ID: "w1"}, workspace.MergeEligibility{}, "", nil))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	folderID, present := decoded["folderId"]
	assert.True(t, present, "folderId must be present even when empty")
	assert.Equal(t, "", folderID)
	order, present := decoded["order"]
	assert.True(t, present, "order must be present even when zero")
	assert.Equal(t, float64(0), order)
}

// TestWorkspaceDTOFrom_ResolvesPlacementFromTheReader pins that FolderID/Order
// come from the placement reader, over the workspace's own id — not from any
// field on domain.Workspace itself, which carries no sidebar position at all
// (see ParentID's own doc: fork lineage only).
func TestWorkspaceDTOFrom_ResolvesPlacementFromTheReader(t *testing.T) {
	reader := fakePlacement{"w1": {folderID: "docs", order: 3}}

	got := dto.WorkspaceDTOFrom(ctx, domain.Workspace{ID: "w1"}, workspace.MergeEligibility{}, "", reader)

	assert.Equal(t, "docs", got.FolderID)
	assert.Equal(t, 3, got.Order)
}

// A workspace the reader has never seen (no Node row yet — see
// dto.WorkspacePlacementReader's own doc) degrades to "" / 0, exactly like a
// nil reader — never an error, and never confused with "" / 0 the reader
// found FOR REAL, which a client cannot and need not tell apart from this.
func TestWorkspaceDTOFrom_UnknownToTheReaderDegradesToZeroValue(t *testing.T) {
	reader := fakePlacement{"w2": {folderID: "docs", order: 3}}

	got := dto.WorkspaceDTOFrom(ctx, domain.Workspace{ID: "w1"}, workspace.MergeEligibility{}, "", reader)

	assert.Equal(t, "", got.FolderID)
	assert.Equal(t, 0, got.Order)
}

// TestWorkspaceDTO_ParentBranchOmitEmpty pins that parentBranch is emitted when
// non-empty and omitted when empty.
func TestWorkspaceDTO_ParentBranchOmitEmpty(
	t *testing.T,
) {
	dtoVal := dto.WorkspaceDTOFrom(ctx, domain.Workspace{ID: "w1"}, workspace.MergeEligibility{}, "", nil)
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
	got := dto.WorkspaceDTOList(ctx, nil, noElig, noOwningChatID, nil)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestWorkspaceDTOList(
	t *testing.T,
) {
	got := dto.WorkspaceDTOList(ctx, []domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	}, noElig, noOwningChatID, nil)
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
	got := dto.WorkspaceDTOList(ctx, []domain.Workspace{
		{ID: "c", CreatedAt: time.Unix(1, 0).UTC()},
		{ID: "b", CreatedAt: time.Unix(3, 0).UTC()},
		{ID: "a", CreatedAt: time.Unix(2, 0).UTC()},
	}, noElig, noOwningChatID, nil)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"c", "a", "b"}, []string{got[0].ID, got[1].ID, got[2].ID})
}

// TestWorkspaceDTOFrom_NeverLeaksAFolderIntoTheForkLineage pins that ParentID
// stays the fork lineage alone — three git paths resolve it back to a
// workspace — even now that FolderID/Order are back on this resource
// (2026-09-09 sidebar-placement-unification, workspace-placement fix): a
// SEPARATE field, resolved from the placement reader, never conflated with
// domain.Workspace.ParentID.
func TestWorkspaceDTOFrom_NeverLeaksAFolderIntoTheForkLineage(
	t *testing.T,
) {
	got := dto.WorkspaceDTOFrom(ctx, domain.Workspace{
		ID: "w1", ParentID: "",
	}, workspace.MergeEligibility{}, "", fakePlacement{"w1": {folderID: "docs", order: 1}})

	assert.Empty(t, got.ParentID, "a folder id must never leak into the fork lineage")
	assert.Equal(t, "docs", got.FolderID, "the placement reader's folder answers FolderID, never ParentID")
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
	got := dto.WorkspaceDTOList(ctx, []domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	}, eligFn, noOwningChatID, nil)
	require.Len(t, got, 2)
	assert.True(t, got[0].CanMergeLocally)
	assert.Equal(t, "main", got[0].ParentBranch)
	assert.False(t, got[1].CanMergeLocally)
	assert.Equal(t, "", got[1].ParentBranch)
}

// TestWorkspaceDTOList_AppliesOwningChatIDFn pins that the per-row
// owning-chat-id resolver is invoked and its result mapped onto each DTO,
// mirroring TestWorkspaceDTOList_AppliesEligFn's shape for the new field.
func TestWorkspaceDTOList_AppliesOwningChatIDFn(
	t *testing.T,
) {
	owningChatIDFn := func(w domain.Workspace) string {
		if w.ID == "w1" {
			return "chat-1"
		}
		return ""
	}
	got := dto.WorkspaceDTOList(ctx, []domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	}, noElig, owningChatIDFn, nil)
	require.Len(t, got, 2)
	assert.Equal(t, "chat-1", got[0].OwningChatID)
	assert.Equal(t, "", got[1].OwningChatID)
}

// TestWorkspaceDTOList_AppliesPlacementReader mirrors
// TestWorkspaceDTOList_AppliesEligFn's shape for the placement reader: each
// row's own FolderID/Order is resolved from it, over that row's own id.
func TestWorkspaceDTOList_AppliesPlacementReader(
	t *testing.T,
) {
	reader := fakePlacement{"w1": {folderID: "docs", order: 5}}
	got := dto.WorkspaceDTOList(ctx, []domain.Workspace{
		{ID: "w1"},
		{ID: "w2"},
	}, noElig, noOwningChatID, reader)
	require.Len(t, got, 2)
	assert.Equal(t, "docs", got[0].FolderID)
	assert.Equal(t, 5, got[0].Order)
	assert.Equal(t, "", got[1].FolderID)
	assert.Equal(t, 0, got[1].Order)
}

func TestWorkspaceDTOFrom_MapsIsDefault(t *testing.T) {
	got := dto.WorkspaceDTOFrom(
		ctx,
		domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", IsDefault: true},
		workspace.MergeEligibility{}, "", nil,
	)
	assert.True(t, got.IsDefault)

	got2 := dto.WorkspaceDTOFrom(
		ctx,
		domain.Workspace{ID: "w2", RepoID: "r1", ProjectID: "p1"},
		workspace.MergeEligibility{}, "", nil,
	)
	assert.False(t, got2.IsDefault)
}

// TestWorkspaceDTOFrom_MapsHeldByPath proves the placeholder holder path
// (domain.Workspace.HeldByPath) is carried onto the wire DTO so the FE can
// reconstruct the placeholder reason from it (spec §4/B3).
func TestWorkspaceDTOFrom_MapsHeldByPath(t *testing.T) {
	got := dto.WorkspaceDTOFrom(
		ctx,
		domain.Workspace{ID: "w1", HeldByPath: "/Users/me/proj"},
		workspace.MergeEligibility{}, "", nil,
	)
	assert.Equal(t, "/Users/me/proj", got.HeldByPath)
}
