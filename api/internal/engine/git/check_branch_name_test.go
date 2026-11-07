package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestCheckBranchName_TakesWhatGitTakes(t *testing.T) {
	repo := initRepo(t)
	e := git.New()
	for _, name := range []string{"feature-x", "fix/login", "chat-1a2b3c4d"} {
		assert.NoError(t, e.CheckBranchName(context.Background(), repo, name), name)
	}
}

func TestCheckBranchName_RefusesWhatGitRefusesOrRewrites(t *testing.T) {
	repo := initRepo(t)
	e := git.New()
	for _, name := range []string{"bad name", "a..b", "x.lock", "HEAD", "-x", "@{-1}", "a~1", "trailing/"} {
		err := e.CheckBranchName(context.Background(), repo, name)
		assert.Equal(t, problem.InvalidArgument, problem.CodeOf(err), name)
		assert.Contains(t, problem.Of(err).Message, "is not a valid branch name", name)
	}
}
