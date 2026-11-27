package git_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestWorkingTreeSummary_DiffsAgainstMergeBaseNotStaleForkPoint(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	makeCommit(t, dir, "base.txt", "base\n", "base")
	forkPoint := headSHA(t, dir)

	bigFile := strings.Repeat("line\n", 100)
	makeCommit(t, dir, "big.txt", bigFile, "advance base branch by 100 lines")

	gitRun(t, dir, "checkout", "-b", "feature", forkPoint)
	makeCommit(t, dir, "feature.txt", "child change\n", "child change")
	gitRun(t, dir, "rebase", "main")

	e := git.New()

	branchAdded, branchDeleted, _, branchHasCommits, err := e.WorkingTreeSummary(ctx, dir, "main")
	require.NoError(t, err)

	staleAdded, _, _, _, err := e.WorkingTreeSummary(ctx, dir, forkPoint)
	require.NoError(t, err)

	assert.Equal(t, 1, branchAdded, "base-branch summary must count only the child's own additions")
	assert.Equal(t, 0, branchDeleted, "base-branch summary must not count the base branch's advancement")
	assert.True(t, branchHasCommits, "the child has its own commit ahead of the merge-base")

	assert.GreaterOrEqual(t, staleAdded, 100, "the stale fork point inflates the diff by the base branch's advancement")
	assert.Less(t, branchAdded, staleAdded, "diffing against the base branch must be smaller than against the stale fork point")
}

func TestWorkingTreeSummary_LocalBaseAheadOfOrigin(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	bigFile := strings.Repeat("line\n", 200)
	makeCommit(t, repo, "big.txt", bigFile, "advance LOCAL main by 200 lines (unpushed)")

	gitRun(t, repo, "checkout", "-b", "feature")
	makeCommit(t, repo, "feature.txt", "child change\n", "child change")

	e := git.New()
	added, deleted, _, hasCommits, err := e.WorkingTreeSummary(ctx, repo, "main")
	require.NoError(t, err)

	assert.Equal(t, 1, added, "must diff against the LOCAL merge-base, not origin's older tip (which would add the 200 unpushed lines)")
	assert.Equal(t, 0, deleted)
	assert.True(t, hasCommits)
}

func TestWorkingTreeSummary_LocalBaseBehindOrigin(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)
	baseSHA := headSHA(t, repo)

	bigFile := strings.Repeat("line\n", 200)
	makeCommit(t, repo, "big.txt", bigFile, "advance main by 200 lines")
	gitRun(t, repo, "push", "origin", "main")
	gitRun(t, repo, "reset", "--hard", baseSHA)
	gitRun(t, repo, "fetch", "origin")

	gitRun(t, repo, "checkout", "-b", "feature", "origin/main")
	makeCommit(t, repo, "feature.txt", "child change\n", "child change")

	e := git.New()
	added, deleted, _, hasCommits, err := e.WorkingTreeSummary(ctx, repo, "main")
	require.NoError(t, err)

	assert.Equal(t, 1, added, "must diff against origin's merge-base, not the stale local main ref (which would add the 200 lines)")
	assert.Equal(t, 0, deleted)
	assert.True(t, hasCommits)
}
