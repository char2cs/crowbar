package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestCreateWorkspace_Validate_RejectsExisting(t *testing.T) {
	cmd := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1"}
	err := cmd.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateWorkspace_Validate_RejectsMissingIDs(t *testing.T) {
	cmd := CreateWorkspace{ID: "w1"}
	err := cmd.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateWorkspace_EmitEvent_SeedsNewStatusAndDefaultStrategy(t *testing.T) {
	now := time.Unix(1000, 0)
	cmd := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: now}
	ws := cmd.EmitEvent(nil)
	assert.Equal(t, domain.WorkspaceStatusNew, ws.Status)
	assert.Equal(t, gitdomain.MergeStrategyMerge, ws.MergeStrategy)
	assert.Equal(t, now, ws.CreatedAt)
}

func TestCreate_SeedsLockedStatusWhenProtected(t *testing.T) {
	now := time.Unix(1000, 0)
	locked := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Locked: true, Now: now}.EmitEvent(nil)
	assert.Equal(t, domain.WorkspaceStatusLocked, locked.Status)
	assert.True(t, locked.Locked)

	unlocked := CreateWorkspace{ID: "w2", RepoID: "r1", ProjectID: "p1", Now: now}.EmitEvent(nil)
	assert.Equal(t, domain.WorkspaceStatusNew, unlocked.Status)
	assert.False(t, unlocked.Locked)
}

func TestSyncWorkingTreeState_Validate_RejectsMissing(t *testing.T) {
	err := SyncWorkingTreeState{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSyncWorkingTreeState_KeepsNewWhenHasCommits(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	ws := SyncWorkingTreeState{ID: "w1", HasCommits: true}.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusNew, ws.Status)
}

func TestSyncWorkingTree_LocalConflictSetsPRConflicts(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	ws := SyncWorkingTreeState{ID: "w1", HasConflicts: true}.EmitEvent(cur)
	assert.True(t, ws.HasConflicts)
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, ws.Status)
}

func TestSyncWorkingTreeState_DoesNotStompPROpen(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPROpen}
	ws := SyncWorkingTreeState{ID: "w1", HasCommits: true}.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestSyncWorkingTreeState_ClampsNegativeCounts(t *testing.T) {
	cur := &domain.Workspace{ID: "w1"}
	ws := SyncWorkingTreeState{ID: "w1", Added: -5, Deleted: -2}.EmitEvent(cur)
	assert.Equal(t, 0, ws.Added)
	assert.Equal(t, 0, ws.Deleted)
}

func TestCommands_Metadata(t *testing.T) {
	c := CreateWorkspace{ID: "w1"}
	require.Equal(t, "w1", c.AggregateID())
	assert.Contains(t, c.EventName(), "workspace.created")
	assert.True(t, c.ShouldSnapshot())
	s := SyncWorkingTreeState{ID: "w1"}
	assert.Equal(t, "w1", s.AggregateID())
	assert.Contains(t, s.EventName(), "working_tree_synced")
	assert.True(t, s.ShouldSnapshot())

	sp := SyncProviderState{ID: "w1"}
	assert.Equal(t, "w1", sp.AggregateID())
	assert.Contains(t, sp.EventName(), "provider_synced")
	assert.True(t, sp.ShouldSnapshot())

	ms := SetMergeStrategy{ID: "w1"}
	assert.Equal(t, "w1", ms.AggregateID())
	assert.Contains(t, ms.EventName(), "merge_strategy_set")
	assert.False(t, ms.ShouldSnapshot())

	ta := TouchActivity{ID: "w1"}
	assert.Equal(t, "w1", ta.AggregateID())
	assert.Contains(t, ta.EventName(), "activity_touched")
	assert.False(t, ta.ShouldSnapshot())

	rp := Reparent{ID: "w1"}
	assert.Equal(t, "w1", rp.AggregateID())
	assert.Contains(t, rp.EventName(), "reparented")
	assert.True(t, rp.ShouldSnapshot())

	ufp := UpdateForkPoint{ID: "w1"}
	assert.Equal(t, "w1", ufp.AggregateID())
	assert.Contains(t, ufp.EventName(), "fork_point_updated")
	assert.False(t, ufp.ShouldSnapshot())

	spm := SetPendingMerge{ID: "w1"}
	assert.Equal(t, "w1", spm.AggregateID())
	assert.Contains(t, spm.EventName(), "pending_merge_set")
	assert.False(t, spm.ShouldSnapshot())

	cpm := ClearPendingMerge{ID: "w1"}
	assert.Equal(t, "w1", cpm.AggregateID())
	assert.Contains(t, cpm.EventName(), "pending_merge_cleared")
	assert.False(t, cpm.ShouldSnapshot())
}

func TestCreateWorkspace_Validate_AcceptsValidNew(t *testing.T) {
	cmd := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1"}
	err := cmd.Validate(nil)
	assert.NoError(t, err)
}

func TestSyncWorkingTreeState_Validate_AcceptsExisting(t *testing.T) {
	cmd := SyncWorkingTreeState{ID: "w1"}
	err := cmd.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestCreateWorkspace_EmitEvent_UsesProvidedStrategy(t *testing.T) {
	now := time.Unix(2000, 0)
	cmd := CreateWorkspace{
		ID:            "w1",
		RepoID:        "r1",
		ProjectID:     "p1",
		MergeStrategy: gitdomain.MergeStrategySquash,
		Now:           now,
	}
	ws := cmd.EmitEvent(nil)
	assert.Equal(t, gitdomain.MergeStrategySquash, ws.MergeStrategy)
	assert.Equal(t, now, ws.LastActivity)
}

func TestSyncProvider_ProtectedSetsLocked_PrStatusOtherwise(t *testing.T) {
	// Protected wins (D4 precedence): Status=locked even with an open PR; PR fields still recorded.
	curProtected := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	cmd := SyncProviderState{
		ID:             "w1",
		Protected:      true,
		HasPR:          true,
		PRStatus:       "open",
		PRUrl:          "https://x/pr/1",
		PRTitle:        "t",
		PRTargetBranch: "main",
		Now:            time.Unix(3000, 0),
	}
	ws := cmd.EmitEvent(curProtected)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status)
	assert.True(t, ws.Locked)
	assert.Equal(t, "https://x/pr/1", ws.PRUrl)
	assert.Equal(t, "main", ws.PRTargetBranch)

	// Not protected: PR status maps to pr-*.
	curOpen := &domain.Workspace{ID: "w2", Status: domain.WorkspaceStatusNew}
	wsOpen := SyncProviderState{ID: "w2", Protected: false, HasPR: true, PRStatus: "open"}.EmitEvent(curOpen)
	assert.Equal(t, domain.WorkspaceStatusPROpen, wsOpen.Status)
	assert.False(t, wsOpen.Locked)
}

func TestSyncProvider_Precedence_DoesNotClobberLocked(t *testing.T) {
	// pr-* must not overwrite an existing locked status.
	curLocked := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusLocked}
	ws := SyncProviderState{ID: "w1", Protected: false, HasPR: true, PRStatus: "open"}.EmitEvent(curLocked)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status)

	// pr-* must not overwrite an existing pr-conflicts status.
	curConflicts := &domain.Workspace{ID: "w2", Status: domain.WorkspaceStatusPRConflicts}
	wsc := SyncProviderState{ID: "w2", Protected: false, HasPR: true, PRStatus: "merged"}.EmitEvent(curConflicts)
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, wsc.Status)

	// deleted is highest precedence.
	curDeleted := &domain.Workspace{ID: "w3", Status: domain.WorkspaceStatusDeleted}
	wsd := SyncProviderState{ID: "w3", Protected: true, HasPR: true, PRStatus: "open"}.EmitEvent(curDeleted)
	assert.Equal(t, domain.WorkspaceStatusDeleted, wsd.Status)
}

func TestSyncProviderState_NoPRLeavesStatusButSetsLocked(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	cmd := SyncProviderState{ID: "w1", Protected: true, HasPR: false}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status)
	assert.True(t, ws.Locked)
	assert.Empty(t, ws.PRUrl)
}

func TestSyncProviderState_PRClosedToOpenAllowed(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPRClosed}
	cmd := SyncProviderState{ID: "w1", HasPR: true, PRStatus: "open"}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestSyncProviderState_Validate_RejectsMissing(t *testing.T) {
	err := SyncProviderState{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSyncProviderState_PRMergedStatus(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPROpen}
	cmd := SyncProviderState{ID: "w1", HasPR: true, PRStatus: "merged"}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPRMerged, ws.Status)
}

func TestSyncProviderState_PRClosedStatus(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPROpen}
	cmd := SyncProviderState{ID: "w1", HasPR: true, PRStatus: "closed"}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPRClosed, ws.Status)
}

func TestSyncProviderState_PRUnknownStatusDefaultsToOpen(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	cmd := SyncProviderState{ID: "w1", HasPR: true, PRStatus: "unknown-future-status"}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestSyncProviderState_Validate_AcceptsExisting(t *testing.T) {
	err := SyncProviderState{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestSetMergeStrategy_OnlyWritesStrategy(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Added: 5, MergeStrategy: gitdomain.MergeStrategyMerge}
	ws := SetMergeStrategy{ID: "w1", Strategy: gitdomain.MergeStrategyRebase}.EmitEvent(cur)
	assert.Equal(t, gitdomain.MergeStrategyRebase, ws.MergeStrategy)
	assert.Equal(t, 5, ws.Added)
}

func TestSetMergeStrategy_Validate_RejectsMissing(t *testing.T) {
	err := SetMergeStrategy{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestTouchActivity_OnlyBumpsLastActivity(t *testing.T) {
	now := time.Unix(9000, 0)
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPROpen}
	ws := TouchActivity{ID: "w1", Now: now}.EmitEvent(cur)
	assert.Equal(t, now, ws.LastActivity)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestTouchActivity_Validate_RejectsMissing(t *testing.T) {
	err := TouchActivity{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSetMergeStrategy_Validate_AcceptsExisting(t *testing.T) {
	err := SetMergeStrategy{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestTouchActivity_Validate_AcceptsExisting(t *testing.T) {
	err := TouchActivity{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestReparent_SetsParentAndForkPoint(t *testing.T) {
	now := time.Unix(4000, 0)
	cur := &domain.Workspace{ID: "w1", ParentID: "old", ForkPointSha: "oldsha"}
	ws := Reparent{ID: "w1", ParentID: "new", ForkPointSha: "newsha", Now: now}.EmitEvent(cur)
	assert.Equal(t, "new", ws.ParentID)
	assert.Equal(t, "newsha", ws.ForkPointSha)
	assert.Equal(t, now, ws.LastActivity)
}

func TestReparent_Validate_RejectsMissingParent(t *testing.T) {
	err := Reparent{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestReparent_Validate_RejectsMissingCurrent(t *testing.T) {
	err := Reparent{ID: "w1", ParentID: "p", ForkPointSha: "s"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateForkPoint_OnlyWritesForkPoint(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", ForkPointSha: "a", ParentID: "p"}
	ws := UpdateForkPoint{ID: "w1", ForkPointSha: "b"}.EmitEvent(cur)
	assert.Equal(t, "b", ws.ForkPointSha)
	assert.Equal(t, "p", ws.ParentID)
}

func TestUpdateForkPoint_Validate_RejectsMissingCurrent(t *testing.T) {
	err := UpdateForkPoint{ID: "w1", ForkPointSha: "s"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateForkPoint_Validate_RejectsMissingSha(t *testing.T) {
	err := UpdateForkPoint{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSetPendingMerge_SetsMarker(t *testing.T) {
	cur := &domain.Workspace{ID: "w1"}
	ws := SetPendingMerge{
		ID:             "w1",
		Strategy:       gitdomain.MergeStrategyRebase,
		TargetParentID: "p",
	}.EmitEvent(cur)
	require.NotNil(t, ws.PendingMerge)
	assert.Equal(t, "p", ws.PendingMerge.TargetParentID)
	assert.Equal(t, gitdomain.MergeStrategyRebase, ws.PendingMerge.Strategy)
}

func TestSetPendingMerge_Validate_RejectsMissingCurrent(t *testing.T) {
	err := SetPendingMerge{ID: "w1", TargetParentID: "p"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSetPendingMerge_Validate_RejectsMissingTarget(t *testing.T) {
	err := SetPendingMerge{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestClearPendingMerge_ClearsMarker(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", PendingMerge: &gitdomain.PendingMerge{TargetParentID: "p"}}
	ws := ClearPendingMerge{ID: "w1"}.EmitEvent(cur)
	assert.Nil(t, ws.PendingMerge)
}

func TestClearPendingMerge_Validate_RejectsMissing(t *testing.T) {
	err := ClearPendingMerge{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestClearPendingMerge_Validate_AcceptsExisting(t *testing.T) {
	err := ClearPendingMerge{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestReparent_Validate_RejectsMissingForkPoint(t *testing.T) {
	err := Reparent{ID: "w1", ParentID: "p"}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestReparent_Validate_AcceptsValid(t *testing.T) {
	err := Reparent{ID: "w1", ParentID: "p", ForkPointSha: "s"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestUpdateForkPoint_Validate_AcceptsValid(t *testing.T) {
	err := UpdateForkPoint{ID: "w1", ForkPointSha: "s"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestSetPendingMerge_Validate_AcceptsValid(t *testing.T) {
	err := SetPendingMerge{ID: "w1", TargetParentID: "p"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}
