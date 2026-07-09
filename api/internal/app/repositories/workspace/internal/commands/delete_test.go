package commands

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestDelete_EmitsTombstone(t *testing.T) {
	cmd := Delete{ID: "ws-1"}
	require.Equal(t, "workspace.deleted.ws-1", cmd.EventName())
	next := cmd.EmitEvent(&domain.Workspace{ID: "ws-1", Status: domain.WorkspaceStatusPROpen})
	require.Equal(t, domain.WorkspaceStatusDeleted, next.Status)
}

func TestDelete_Metadata(t *testing.T) {
	c := Delete{ID: "ws-1"}
	assert.Equal(t, "ws-1", c.AggregateID())
	assert.Contains(t, c.EventName(), "workspace.deleted")
	assert.False(t, c.ShouldSnapshot())
}

func TestDelete_Validate_RejectsMissing(t *testing.T) {
	err := Delete{ID: "ws-1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestDelete_Validate_AcceptsExisting(t *testing.T) {
	err := Delete{ID: "ws-1"}.Validate(&domain.Workspace{ID: "ws-1"})
	assert.NoError(t, err)
}

func TestDelete_EmitEvent_PreservesLocationFields(t *testing.T) {
	cur := &domain.Workspace{
		ID:           "ws-1",
		RepoID:       "r1",
		ProjectID:    "p1",
		WorktreePath: "/h/projects/p/github.com/o/r/main",
		Status:       domain.WorkspaceStatusPROpen,
	}
	next := Delete{ID: "ws-1"}.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusDeleted, next.Status)
	assert.Equal(t, "r1", next.RepoID)
	assert.Equal(t, "p1", next.ProjectID)
	assert.Equal(t, "/h/projects/p/github.com/o/r/main", next.WorktreePath)
}
