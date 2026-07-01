package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestClearBranch_BlanksBranchOnly(t *testing.T) {
	cur := &domain.Workspace{
		ID: "home", Branch: "develop", WorktreePath: "/repo",
		Status: domain.WorkspaceStatusNew, IsDefault: true,
	}
	got := commands.ClearBranch{ID: "home"}.EmitEvent(cur)
	assert.Empty(t, got.Branch, "branch is blanked")
	assert.Equal(t, "/repo", got.WorktreePath, "worktree path is untouched")
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status, "status is untouched")
	assert.True(t, got.IsDefault, "identity is untouched (chats/threads preserved)")
}

func TestClearBranch_Validate_RejectsMissing(t *testing.T) {
	err := commands.ClearBranch{ID: "home"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
