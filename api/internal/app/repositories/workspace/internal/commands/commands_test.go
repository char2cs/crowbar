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

func TestCreate_SeedsLockedWhenProtected(t *testing.T) {
	now := time.Unix(1000, 0)
	locked := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Protected: true, Now: now}.EmitEvent(nil)
	assert.Equal(t, domain.WorkspaceStatusLocked, locked.Status)

	unlocked := CreateWorkspace{ID: "w2", RepoID: "r1", ProjectID: "p1", Now: now}.EmitEvent(nil)
	assert.Equal(t, domain.WorkspaceStatusNew, unlocked.Status)
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
}

func TestSetLastError_SetsMessage(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	ws := SetLastError{ID: "w1", Message: "boom"}.EmitEvent(cur)
	assert.Equal(t, "boom", ws.LastError)
	assert.Equal(t, domain.WorkspaceStatusNew, ws.Status)
}

func TestSetLastError_Validate_RejectsMissing(t *testing.T) {
	err := SetLastError{ID: "w1", Message: "x"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSetLastError_Validate_AcceptsExisting(t *testing.T) {
	err := SetLastError{ID: "w1", Message: "x"}.Validate(&domain.Workspace{ID: "w1"})
	assert.NoError(t, err)
}

func TestSetLastError_Metadata(t *testing.T) {
	c := SetLastError{ID: "w1"}
	assert.Equal(t, "w1", c.AggregateID())
	assert.Contains(t, c.EventName(), "last_error_set")
	assert.False(t, c.ShouldSnapshot())
}

func TestCreate_ClearsLastError(t *testing.T) {
	ws := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Empty(t, ws.LastError)
}

func TestSyncWorkingTreeState_ClearsLastError(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew, LastError: "stale"}
	ws := SyncWorkingTreeState{ID: "w1", HasCommits: true}.EmitEvent(cur)
	assert.Empty(t, ws.LastError)
}

func TestSyncProviderState_ClearsLastError(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew, LastError: "stale"}
	ws := SyncProviderState{ID: "w1", HasPR: true, PRStatus: "open"}.EmitEvent(cur)
	assert.Empty(t, ws.LastError)
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
	assert.Equal(t, "https://x/pr/1", ws.PRUrl)
	assert.Equal(t, "main", ws.PRTargetBranch)

	// Not protected: PR status maps to pr-*.
	curOpen := &domain.Workspace{ID: "w2", Status: domain.WorkspaceStatusNew}
	wsOpen := SyncProviderState{ID: "w2", Protected: false, HasPR: true, PRStatus: "open"}.EmitEvent(curOpen)
	assert.Equal(t, domain.WorkspaceStatusPROpen, wsOpen.Status)
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

func TestCreateWorkspace_EmitEvent_KindDefault(t *testing.T) {
	cmd := CreateWorkspace{
		ID:        "ws-1",
		RepoID:    "repo-1",
		ProjectID: "proj-1",
		Branch:    "main",
		Now:       time.Now(),
		// Kind not set → should default to git
	}
	ws := cmd.EmitEvent(nil)
	require.Equal(t, domain.WorkspaceKindGit, ws.Kind)
}

func TestCreateWorkspace_EmitEvent_KindHome(t *testing.T) {
	cmd := CreateWorkspace{
		ID:        "ws-home",
		ProjectID: "proj-1",
		Kind:      domain.WorkspaceKindHome,
		Now:       time.Now(),
	}
	ws := cmd.EmitEvent(nil)
	require.Equal(t, domain.WorkspaceKindHome, ws.Kind)
	require.Empty(t, ws.RepoID)
}

func TestCreateWorkspace_Validate_HomeAllowsEmptyRepoID(t *testing.T) {
	cmd := CreateWorkspace{
		ID:        "ws-home",
		ProjectID: "proj-1",
		Kind:      domain.WorkspaceKindHome,
	}
	require.NoError(t, cmd.Validate(nil))
}

func TestCreateWorkspace_Validate_GitRequiresRepoID(t *testing.T) {
	cmd := CreateWorkspace{
		ID:        "ws-git",
		ProjectID: "proj-1",
		Kind:      domain.WorkspaceKindGit,
	}
	require.Error(t, cmd.Validate(nil))
}
