package diff_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// countCommittedQueries wraps the seam the cached ref->HEAD numstat reaches git
// through and returns a pointer to its invocation count. Every assertion below
// is made on that count: a cache hit is an invocation that did not happen, not
// a call that finished quickly, so nothing here depends on timing.
func countCommittedQueries(
	t *testing.T,
) *int {
	t.Helper()
	diff.ResetSummaryCache()
	calls := 0
	var real diff.GitRunner
	real, restore := diff.SetGitRunner(func(ctx context.Context, dir string, args ...string) exec.Result {
		calls++
		return real(ctx, dir, args...)
	})
	t.Cleanup(restore)
	t.Cleanup(diff.ResetSummaryCache)
	return &calls
}

func summariesOf(
	t *testing.T,
	repo string,
	ref string,
) int {
	t.Helper()
	files, err := diff.FileSummaries(context.Background(), repo, ref, dirtyPaths(t, repo))
	require.NoError(t, err)
	return len(files)
}

func seedCacheRepo(
	t *testing.T,
) (string, string) {
	t.Helper()
	repo := initRepo(t)
	writeFile(t, repo, "a.txt", "1\n2\n3\n4\n5\n")
	writeFile(t, repo, "b.txt", "x\ny\n")
	commitAll(t, repo, "base")
	ref := headSHA(t, repo)
	writeFile(t, repo, "a.txt", "1\n2\n3\n4\n5\n6\n7\n")
	commitAll(t, repo, "committed work")
	return repo, ref
}

// TestSummaryCache_HitsWhenOnlyWorktreeChanged is the case the split exists
// for: while an agent churns the tree, every tick re-reads the same two commits
// and only the working tree has moved, so the expensive half must run once.
func TestSummaryCache_HitsWhenOnlyWorktreeChanged(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, ref := seedCacheRepo(t)

	require.Positive(t, summariesOf(t, repo, ref))
	require.Equal(t, 1, *calls, "cold cache runs the committed query once")

	writeFile(t, repo, "a.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	require.Positive(t, summariesOf(t, repo, ref))
	assert.Equal(t, 1, *calls, "a working-tree edit must not re-run the committed query")

	writeFile(t, repo, "b.txt", "x\ny\nz\n")
	writeFile(t, repo, "untracked.txt", "u\n")
	require.Positive(t, summariesOf(t, repo, ref))
	assert.Equal(t, 1, *calls, "more churn, still one committed query")
}

func TestSummaryCache_MissesAfterCommit(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, ref := seedCacheRepo(t)

	summariesOf(t, repo, ref)
	require.Equal(t, 1, *calls)

	writeFile(t, repo, "b.txt", "x\ny\nz\n")
	commitAll(t, repo, "second commit")

	summariesOf(t, repo, ref)
	assert.Equal(t, 2, *calls, "a new HEAD is a new key")
}

func TestSummaryCache_MissesAfterRefChange(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, firstRef := seedCacheRepo(t)
	secondRef := headSHA(t, repo)

	summariesOf(t, repo, firstRef)
	require.Equal(t, 1, *calls)

	summariesOf(t, repo, secondRef)
	assert.Equal(t, 2, *calls, "a different base ref is a different key")

	summariesOf(t, repo, firstRef)
	assert.Equal(t, 2, *calls, "and the first key is still cached")
}

func TestSummaryCache_MissesAfterReset(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, ref := seedCacheRepo(t)
	writeFile(t, repo, "b.txt", "x\ny\nz\n")
	commitAll(t, repo, "second commit")

	summariesOf(t, repo, ref)
	require.Equal(t, 1, *calls)

	mustGit(t, repo, "reset", "--hard", "HEAD~1")

	summariesOf(t, repo, ref)
	assert.Equal(t, 2, *calls, "HEAD moving backwards is a different key, not a stale hit")
}

func TestSummaryCache_IsPerRepo(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, ref := seedCacheRepo(t)

	// A clone shares the source's commit SHAs, so the two keys differ in the
	// repo path and nothing else — exactly what this asserts is part of the key.
	clone := filepath.Join(t.TempDir(), "clone")
	mustGit(t, repo, "clone", repo, clone)

	first := summariesOf(t, repo, ref)
	require.Equal(t, 1, *calls)

	second := summariesOf(t, clone, ref)
	assert.Equal(t, 2, *calls, "two repos must not share cache entries")
	assert.Equal(t, first, second)
}

// TestSummaryCache_BoundedSize pins the eviction: a session moving across many
// base refs and commits must not grow the cache without limit.
func TestSummaryCache_BoundedSize(t *testing.T) {
	countCommittedQueries(t)
	repo := initRepo(t)
	writeFile(t, repo, "a.txt", "0\n")
	commitAll(t, repo, "base")

	refs := make([]string, 0, diff.SummaryCacheCap+8)
	for i := range diff.SummaryCacheCap + 8 {
		refs = append(refs, headSHA(t, repo))
		writeFile(t, repo, "a.txt", fmt.Sprintf("%d\n", i+1))
		commitAll(t, repo, fmt.Sprintf("commit %d", i))
	}
	for _, ref := range refs {
		summariesOf(t, repo, ref)
	}

	assert.Equal(t, diff.SummaryCacheCap, diff.SummaryCacheSize(),
		"the cache must evict rather than grow past its cap")
}

// TestSummaryCache_ColdEntryStillCorrect guards the eviction path: a key pushed
// out and asked for again must recompute rather than return an empty map.
func TestSummaryCache_ColdEntryStillCorrect(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, ref := seedCacheRepo(t)

	want := summariesOf(t, repo, ref)
	require.Equal(t, 1, *calls)

	diff.ResetSummaryCache()

	assert.Equal(t, want, summariesOf(t, repo, ref))
	assert.Equal(t, 2, *calls)
}
