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

func TestForkPoint_FindsTheForkOfARewrittenUpstream(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "base")
	gitRun(t, dir, "checkout", "-b", "parent")
	makeCommit(t, dir, "p.txt", "p\n", "parent work")
	fork := headSHA(t, dir)
	gitRun(t, dir, "checkout", "-b", "child")
	makeCommit(t, dir, "c.txt", "c\n", "child work")
	gitRun(t, dir, "checkout", "parent")
	makeCommit(t, dir, "p.txt", "p rewritten\n", "parent work, amended")
	gitRun(t, dir, "reset", "--soft", "HEAD~2")
	gitRun(t, dir, "commit", "-m", "parent work, squashed")

	got, err := git.New().ForkPoint(ctx, dir, "parent", "child")

	require.NoError(t, err)
	assert.Equal(t, fork, got, "the parent's reflog still names the commit the child forked from")
}

func TestForkPoint_IsEmptyWithoutAReflog(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "base")
	gitRun(t, dir, "checkout", "-b", "parent")
	makeCommit(t, dir, "p.txt", "p\n", "parent work")
	gitRun(t, dir, "checkout", "-b", "child")
	makeCommit(t, dir, "c.txt", "c\n", "child work")
	gitRun(t, dir, "checkout", "parent")
	gitRun(t, dir, "commit", "--amend", "-m", "parent work, amended", "--allow-empty")
	gitRun(t, dir, "reflog", "expire", "--expire=now", "--all")

	got, err := git.New().ForkPoint(ctx, dir, "parent", "child")

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestForkPoint_UnknownRefErrors(
	t *testing.T,
) {
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	_, err := git.New().ForkPoint(context.Background(), dir, "main", "does-not-exist")

	assert.Error(t, err)
}
