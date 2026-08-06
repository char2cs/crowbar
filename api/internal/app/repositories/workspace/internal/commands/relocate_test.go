package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRelocate_MovesOnlyThePath(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "feature/x", WorktreePath: "/old/worktree",
		Status: domain.WorkspaceStatusNew, ParentID: "p", ForkPointSha: "abc",
		LastError: "stale",
	}

	got := commands.Relocate{ID: "w1", WorktreePath: "/new/worktree"}.EmitEvent(cur)

	assert.Equal(t, "/new/worktree", got.WorktreePath)
	// Everything that identifies the workspace is untouched — this is the SAME
	// workspace at a new address, which is the whole distinction from a rename.
	assert.Equal(t, "feature/x", got.Branch)
	assert.Equal(t, "p", got.ParentID)
	assert.Equal(t, "abc", got.ForkPointSha)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status)
	assert.Empty(t, got.LastError, "a successful mutation clears the stale error")
}

func TestRelocate_Validate_RejectsMissingAggregate(t *testing.T) {
	err := commands.Relocate{ID: "w1", WorktreePath: "/p"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestRelocate_Validate_RejectsAnEmptyDestination(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", WorktreePath: "/old/worktree"}
	err := commands.Relocate{ID: "w1"}.Validate(cur)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

// A workspace with no worktree is an unprovisioned placeholder or an already
// torn-down record; relocating it would invent a tree it never had.
func TestRelocate_Validate_RejectsAWorkspaceWithNoWorktree(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", WorktreePath: ""}
	err := commands.Relocate{ID: "w1", WorktreePath: "/new/worktree"}.Validate(cur)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestRelocate_Accepts(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", WorktreePath: "/old/worktree"}
	require.NoError(t, commands.Relocate{ID: "w1", WorktreePath: "/new/worktree"}.Validate(cur))
	assert.Equal(t, "w1", commands.Relocate{ID: "w1"}.AggregateID())
	assert.Equal(t, "workspace.relocated.w1", commands.Relocate{ID: "w1"}.EventName())
	assert.True(t, commands.Relocate{ID: "w1"}.ShouldSnapshot())
}
