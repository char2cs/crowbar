package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestProvisionInPlace_SetsPathAndClearsHeldBy(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "develop", Status: domain.WorkspaceStatusLocked,
		HeldByPath: "/repo", WorktreePath: "",
	}
	got := commands.ProvisionInPlace{ID: "w1", WorktreePath: "/managed", ForkPointSha: "sha"}.EmitEvent(cur)
	assert.Equal(t, "/managed", got.WorktreePath)
	assert.Equal(t, "sha", got.ForkPointSha)
	assert.Empty(t, got.HeldByPath, "a successful provision clears the holder path")
	assert.Equal(t, domain.WorkspaceStatusLocked, got.Status, "status stays locked")
	assert.Equal(t, "develop", got.Branch, "branch is untouched")
}

func TestProvisionInPlace_Validate_RejectsMissing(t *testing.T) {
	err := commands.ProvisionInPlace{ID: "w1", WorktreePath: "/m"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
