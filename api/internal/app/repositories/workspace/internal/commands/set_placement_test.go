package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSetPlacement_Validate_RejectsMissingAggregate(t *testing.T) {
	err := SetPlacement{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSetPlacement_Validate_RejectsANegativeOrder(t *testing.T) {
	err := SetPlacement{ID: "w1", Order: -1}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

// The whole point of a separate command: the sidebar writes the folder edge and
// the index, and touches nothing the git paths resolve.
func TestSetPlacement_EmitEvent_LeavesTheForkLineageAlone(t *testing.T) {
	current := domain.Workspace{
		ID: "w1", ParentID: "parent", ForkPointSha: "abc123", Branch: "feat", Order: 0,
	}

	got := SetPlacement{ID: "w1", FolderID: "f1", Order: 3}.EmitEvent(&current)

	assert.Equal(t, "f1", got.FolderID)
	assert.Equal(t, 3, got.Order)
	assert.Equal(t, "parent", got.ParentID, "the fork parent is a git fact, not a sidebar one")
	assert.Equal(t, "abc123", got.ForkPointSha)
	assert.Equal(t, "feat", got.Branch)
}

func TestSetPlacement_EmitEvent_ClearsTheFolder(t *testing.T) {
	current := domain.Workspace{ID: "w1", FolderID: "f1", Order: 3}

	got := SetPlacement{ID: "w1", Order: 0}.EmitEvent(&current)

	assert.Empty(t, got.FolderID, "an empty folder is a move back to the repo root")
	assert.Equal(t, 0, got.Order)
}

func TestSetPlacement_EventNameAndSnapshot(t *testing.T) {
	cmd := SetPlacement{ID: "w1"}

	assert.Equal(t, "w1", cmd.AggregateID())
	assert.Equal(t, "workspace.placement_set.w1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot(), "a placement is a two-field write, not a lineage change")
}

func TestSetPlacement_Validate_AcceptsAZeroOrder(t *testing.T) {
	assert.NoError(t, SetPlacement{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"}))
}
