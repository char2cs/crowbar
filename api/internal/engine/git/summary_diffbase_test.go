package git_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestWorkingTreeSummary_DiffsAgainstMergeBaseNotStaleForkPoint pins the fix for
// the stale-base sidebar bug: when a branch is rebased onto a base branch that
// advanced past the branch's original fork point, the summary must count only
// the branch's OWN changes (the merge-base of the base branch's current tip and
// HEAD), never the base branch's whole advancement.
//
// It builds the exact shape that produced the field bug (+69k/-44k): a base
// branch that gained a large commit after the child forked, a child rebased onto
// that new base, and the child's original fork point left frozen behind. Passing
// the base BRANCH yields the small, correct diff; passing the frozen fork SHA
// (the old behaviour) reproduces the inflated diff — proving the base selection
// is what matters.
func TestWorkingTreeSummary_DiffsAgainstMergeBaseNotStaleForkPoint(
	t *testing.T,
) {
	ctx := context.Background()
	dir := initRepo(t)

	// C0: the base branch tip the child forks from.
	makeCommit(t, dir, "base.txt", "base\n", "base")
	forkPoint := headSHA(t, dir)

	// The base branch (main) advances with a LARGE commit the child does not have.
	bigFile := strings.Repeat("line\n", 100)
	makeCommit(t, dir, "big.txt", bigFile, "advance base branch by 100 lines")

	// The child forks from C0, makes one small change, then is rebased onto the
	// advanced base — so it now CONTAINS the 100-line commit, while its recorded
	// fork point still points at C0 (the frozen, now-stale base).
	gitRun(t, dir, "checkout", "-b", "feature", forkPoint)
	makeCommit(t, dir, "feature.txt", "child change\n", "child change")
	gitRun(t, dir, "rebase", "main")

	e := git.New()

	branchAdded, branchDeleted, _, branchHasCommits, err := e.WorkingTreeSummary(ctx, dir, "main")
	require.NoError(t, err)

	staleAdded, _, _, _, err := e.WorkingTreeSummary(ctx, dir, forkPoint)
	require.NoError(t, err)

	// Correct: diffing against the base branch counts only the child's one-line
	// change, because the merge-base with main's current tip already contains the
	// 100-line commit.
	assert.Equal(t, 1, branchAdded, "base-branch summary must count only the child's own additions")
	assert.Equal(t, 0, branchDeleted, "base-branch summary must not count the base branch's advancement")
	assert.True(t, branchHasCommits, "the child has its own commit ahead of the merge-base")

	// The old behaviour (frozen fork point) counts the base branch's 100-line
	// advancement too — the bug. The fix is that the base-branch diff is far
	// smaller than this stale-fork diff.
	assert.GreaterOrEqual(t, staleAdded, 100, "the stale fork point inflates the diff by the base branch's advancement")
	assert.Less(t, branchAdded, staleAdded, "diffing against the base branch must be smaller than against the stale fork point")
}

// TestWorkingTreeSummary_LocalBaseAheadOfOrigin guards against blindly preferring
// origin/<base>: when the LOCAL base branch is ahead of origin (un-pushed commits)
// and the child is built on the local tip, the summary must diff against the LOCAL
// merge-base — the one closest to HEAD — not origin's older tip, which would
// re-inflate the diff by the base branch's un-pushed advancement.
func TestWorkingTreeSummary_LocalBaseAheadOfOrigin(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t) // main == origin/main at the base commit

	// Local main advances by a large commit that is NEVER pushed: local main is now
	// ahead of origin/main.
	bigFile := strings.Repeat("line\n", 200)
	makeCommit(t, repo, "big.txt", bigFile, "advance LOCAL main by 200 lines (unpushed)")

	// The child is built on the local main tip and adds one line.
	gitRun(t, repo, "checkout", "-b", "feature")
	makeCommit(t, repo, "feature.txt", "child change\n", "child change")

	e := git.New()
	added, deleted, _, hasCommits, err := e.WorkingTreeSummary(ctx, repo, "main")
	require.NoError(t, err)

	assert.Equal(t, 1, added, "must diff against the LOCAL merge-base, not origin's older tip (which would add the 200 unpushed lines)")
	assert.Equal(t, 0, deleted)
	assert.True(t, hasCommits)
}

// TestWorkingTreeSummary_LocalBaseBehindOrigin covers the complementary case: the
// LOCAL base branch lags origin and the child is built on origin's fresher tip.
// The summary must diff against origin's merge-base (the closest to HEAD), not the
// stale local branch ref — which would re-inflate the diff by origin's advancement.
// Together with the local-ahead case this proves resolveDiffBase picks whichever
// of {origin/base, local base} is the true, closest base regardless of which side
// is ahead.
func TestWorkingTreeSummary_LocalBaseBehindOrigin(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)
	baseSHA := headSHA(t, repo)

	// origin/main advances by a large commit; the LOCAL main ref is reset back to
	// the base, so local main now lags origin/main by that commit.
	bigFile := strings.Repeat("line\n", 200)
	makeCommit(t, repo, "big.txt", bigFile, "advance main by 200 lines")
	gitRun(t, repo, "push", "origin", "main")
	gitRun(t, repo, "reset", "--hard", baseSHA)
	gitRun(t, repo, "fetch", "origin")

	// The child is built on origin's fresher tip and adds one line.
	gitRun(t, repo, "checkout", "-b", "feature", "origin/main")
	makeCommit(t, repo, "feature.txt", "child change\n", "child change")

	e := git.New()
	added, deleted, _, hasCommits, err := e.WorkingTreeSummary(ctx, repo, "main")
	require.NoError(t, err)

	assert.Equal(t, 1, added, "must diff against origin's merge-base, not the stale local main ref (which would add the 200 lines)")
	assert.Equal(t, 0, deleted)
	assert.True(t, hasCommits)
}
