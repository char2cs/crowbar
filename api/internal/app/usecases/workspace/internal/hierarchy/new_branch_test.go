package hierarchy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestCheckNewBranch_AFreeValidNameOrNoNamePasses(t *testing.T) {
	h := newRealUsecase(t)
	assert.NoError(t, h.uc.CheckNewBranch(context.Background(), h.parentID, "feature-x"))
	assert.NoError(t, h.uc.CheckNewBranch(context.Background(), h.parentID, ""))
}

func TestCheckNewBranch_RefusesANameGitRejects(t *testing.T) {
	h := newRealUsecase(t)
	err := h.uc.CheckNewBranch(context.Background(), h.parentID, "bad name")
	assert.Equal(t, problem.InvalidArgument, problem.CodeOf(err))
	assert.Equal(t, `"bad name" is not a valid branch name`, problem.Of(err).Message)
}

func TestCheckNewBranch_RefusesABranchTheRepoAlreadyHas(t *testing.T) {
	h := newRealUsecase(t)
	gitRun(t, h.repoPath, "branch", "loose")
	held := h.createChild(t, "held", h.parentID, h.baseBranch)
	gitRun(t, h.repoPath, "push", "origin", h.baseBranch+":refs/heads/upstream-only")
	gitRun(t, h.repoPath, "fetch", "origin")

	for _, name := range []string{"loose", held.Branch, h.baseBranch, "upstream-only"} {
		err := h.uc.CheckNewBranch(context.Background(), h.parentID, name)
		assert.Equal(t, problem.Conflict, problem.CodeOf(err), name)
		assert.Equal(t, `"`+name+`" already exists`, problem.Of(err).Message, name)
	}
}
