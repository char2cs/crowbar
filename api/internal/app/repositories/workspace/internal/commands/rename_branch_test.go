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

func TestRenameBranch_ChangesTheBranchAndNothingElse(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "testing", WorktreePath: "/home/p/repo/testing/worktree",
		Status: domain.WorkspaceStatusNew, ParentID: "parent-1", ForkPointSha: "abc123",
	}

	got := commands.RenameBranch{
		ID: "w1", Branch: "feature/x",
	}.EmitEvent(cur)

	assert.Equal(t, "feature/x", got.Branch)
	// The PATH is untouched. It used to travel with the branch because the leaf
	// directory was named after it; the root is keyed by workspace id now, so a
	// rename moves nothing and Relocate is the only command that changes a path.
	assert.Equal(t, "/home/p/repo/testing/worktree", got.WorktreePath,
		"a rename must not move the workspace")
	// The rename must not disturb identity or lineage: children reference this
	// workspace by ID, and the fork point still describes the same commit.
	assert.Equal(t, "parent-1", got.ParentID, "lineage is untouched")
	assert.Equal(t, "abc123", got.ForkPointSha, "fork point is untouched")
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status, "status is untouched")
}

func TestRenameBranch_Validate_RejectsMissingAggregate(t *testing.T) {
	err := commands.RenameBranch{ID: "w1", Branch: "b"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestRenameBranch_Validate_RejectsEmptyBranch(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Branch: "testing", WorktreePath: "/p"}
	err := commands.RenameBranch{ID: "w1", Branch: ""}.Validate(cur)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation),
		"blanking a branch is ClearBranch's job, not a rename")
}

func TestRenameBranch_Validate_AcceptsARenameThatCarriesNoPath(t *testing.T) {
	// The old command REQUIRED a worktree path and refused a "half-carried"
	// rename. Carrying one is now the error, not the omission.
	cur := &domain.Workspace{ID: "w1", Branch: "testing", WorktreePath: "/p"}
	err := commands.RenameBranch{ID: "w1", Branch: "b"}.Validate(cur)
	require.NoError(t, err)
}

func TestRenameBranch_Identity(t *testing.T) {
	c := commands.RenameBranch{ID: "w1", Branch: "b"}
	assert.Equal(t, "w1", c.AggregateID())
	assert.Equal(t, "workspace.branch_renamed.w1", c.EventName())
	assert.True(t, c.ShouldSnapshot())
}
