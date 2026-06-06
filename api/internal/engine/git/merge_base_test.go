package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestMergeBase_ReturnsCommonAncestor(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	makeCommit(t, dir, "base.txt", "base\n", "fork commit")
	fork := headSHA(t, dir)

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.txt", "feature\n", "feature work")

	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "main.txt", "main\n", "main work")

	e := git.New()
	got, err := e.MergeBase(ctx, dir, "main", "feature")

	require.NoError(t, err)
	assert.Equal(t, fork, got)
}

func TestMergeBase_UnknownRefErrors(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	e := git.New()
	_, err := e.MergeBase(ctx, dir, "main", "does-not-exist")

	assert.Error(t, err)
}
