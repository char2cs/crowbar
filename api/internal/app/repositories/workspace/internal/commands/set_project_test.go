package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSetProject_Validate_RejectsMissingAggregate(t *testing.T) {
	err := SetProject{ID: "w1", ProjectID: "p2"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSetProject_Validate_RejectsABlankProject(t *testing.T) {
	err := SetProject{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

// A repo move re-points the workspace's project and nothing else. Above all it
// does not move the worktree: the path was derived once and is stored absolute,
// so rewriting it here would strand the tree it names.
func TestSetProject_EmitEvent_MovesOnlyTheProject(t *testing.T) {
	current := domain.Workspace{
		ID: "w1", ProjectID: "p1", RepoID: "r1", WorktreePath: "/home/u/.crowbar/projects/p1/repo/feat/worktree",
	}

	got := SetProject{ID: "w1", ProjectID: "p2"}.EmitEvent(&current)

	assert.Equal(t, "p2", got.ProjectID)
	assert.Equal(t, "r1", got.RepoID)
	assert.Equal(t, current.WorktreePath, got.WorktreePath, "the on-disk tree does not move with the record")
}

func TestSetProject_EventNameAndSnapshot(t *testing.T) {
	cmd := SetProject{ID: "w1", ProjectID: "p2"}

	assert.Equal(t, "w1", cmd.AggregateID())
	assert.Equal(t, "workspace.project_set.w1", cmd.EventName())
	assert.True(t, cmd.ShouldSnapshot(), "the owning project is identity, worth a snapshot")
}

func TestSetProject_Validate_AcceptsALiveAggregate(t *testing.T) {
	assert.NoError(t, SetProject{ID: "w1", ProjectID: "p2"}.Validate(&domain.Workspace{ID: "w1"}))
}
