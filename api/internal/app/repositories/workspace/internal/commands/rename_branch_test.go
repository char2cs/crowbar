package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRenameBranch_MovesTheBranchAndLeavesThePathAlone(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "testing", WorktreePath: "/home/p/repo/testing/worktree",
		Status: domain.WorkspaceStatusNew, ParentID: "parent-1", ForkPointSha: "abc123",
	}

	got := commands.RenameBranch{ID: "w1", Branch: "feature/x"}.EmitEvent(cur)

	assert.Equal(t, "feature/x", got.Branch)
	// The directory is fixed at creation and never follows the branch, so the
	// path a rename leaves behind is the one it started with.
	assert.Equal(t, "/home/p/repo/testing/worktree", got.WorktreePath,
		"a rename must not move the workspace")
	// The rename must not disturb identity or lineage: children reference this
	// workspace by ID, and the fork point still describes the same commit.
	assert.Equal(t, "parent-1", got.ParentID, "lineage is untouched")
	assert.Equal(t, "abc123", got.ForkPointSha, "fork point is untouched")
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status, "status is untouched")
}

// TestRenameBranch_RenamesIntoItsOwnNamespace pins the case the old
// move-the-directory rename could not express at all: the destination was a
// child of the source, and the kernel refuses that rename with EINVAL.
func TestRenameBranch_RenamesIntoItsOwnNamespace(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "testing", WorktreePath: "/home/p/repo/testing/worktree",
	}

	got := commands.RenameBranch{ID: "w1", Branch: "testing/x"}.EmitEvent(cur)

	assert.Equal(t, "testing/x", got.Branch)
	assert.Equal(t, "/home/p/repo/testing/worktree", got.WorktreePath)
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
