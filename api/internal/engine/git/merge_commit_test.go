package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestMergeCommit_NeverFastForwards(
	t *testing.T,
) {
	dir := initRepo(t)
	makeCommit(t, dir, "a.txt", "a\n", "base")
	base := headSHA(t, dir)
	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "b.txt", "b\n", "ahead")
	ahead := headSHA(t, dir)
	gitRun(t, dir, "checkout", "main")

	require.NoError(t, git.New().MergeCommit(context.Background(), dir, "feature"))

	assert.Equal(t, base+" "+ahead, gitOutput(t, dir, "log", "-1", "--format=%P"),
		"a branch that could fast-forward still lands as a merge commit")
}
