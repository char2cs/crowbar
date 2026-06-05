package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
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
	assert.Equal(t, domain.MergeStrategyMerge, ws.MergeStrategy)
	assert.Equal(t, now, ws.CreatedAt)
}

func TestSyncWorkingTreeState_Validate_RejectsMissing(t *testing.T) {
	err := SyncWorkingTreeState{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSyncWorkingTreeState_ClearsNewWhenHasCommits(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	ws := SyncWorkingTreeState{ID: "w1", HasCommits: true}.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatus(""), ws.Status)
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
		MergeStrategy: domain.MergeStrategySquash,
		Now:           now,
	}
	ws := cmd.EmitEvent(nil)
	assert.Equal(t, domain.MergeStrategySquash, ws.MergeStrategy)
	assert.Equal(t, now, ws.LastActivity)
}
