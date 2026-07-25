package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRenameBranch_MovesBranchAndWorktreePathTogether(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "testing", WorktreePath: "/home/p/repo/testing/worktree",
		Status: domain.WorkspaceStatusNew, ParentID: "parent-1", ForkPointSha: "abc123",
	}

	got := commands.RenameBranch{
		ID: "w1", Branch: "feature/x", WorktreePath: "/home/p/repo/feature/x/worktree",
	}.EmitEvent(cur)

	assert.Equal(t, "feature/x", got.Branch)
	assert.Equal(t, "/home/p/repo/feature/x/worktree", got.WorktreePath)
	// The rename must not disturb identity or lineage: children reference this
	// workspace by ID, and the fork point still describes the same commit.
	assert.Equal(t, "parent-1", got.ParentID, "lineage is untouched")
	assert.Equal(t, "abc123", got.ForkPointSha, "fork point is untouched")
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status, "status is untouched")
}

func TestRenameBranch_Validate_RejectsMissingAggregate(t *testing.T) {
	err := commands.RenameBranch{ID: "w1", Branch: "b", WorktreePath: "/p"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestRenameBranch_Validate_RejectsEmptyBranch(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Branch: "testing", WorktreePath: "/p"}
	err := commands.RenameBranch{ID: "w1", Branch: "", WorktreePath: "/p"}.Validate(cur)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation),
		"blanking a branch is ClearBranch's job, not a rename")
}

func TestRenameBranch_Validate_RejectsEmptyWorktreePath(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Branch: "testing", WorktreePath: "/p"}
	err := commands.RenameBranch{ID: "w1", Branch: "b", WorktreePath: ""}.Validate(cur)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation),
		"a rename must always carry the relocated worktree path")
}
