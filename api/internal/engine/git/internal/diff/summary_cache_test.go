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

	clone := filepath.Join(t.TempDir(), "clone")
	mustGit(t, repo, "clone", repo, clone)

	first := summariesOf(t, repo, ref)
	require.Equal(t, 1, *calls)

	second := summariesOf(t, clone, ref)
	assert.Equal(t, 2, *calls, "two repos must not share cache entries")
	assert.Equal(t, first, second)
}

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

func TestSummaryCache_ColdEntryStillCorrect(t *testing.T) {
	calls := countCommittedQueries(t)
	repo, ref := seedCacheRepo(t)

	want := summariesOf(t, repo, ref)
	require.Equal(t, 1, *calls)

	diff.ResetSummaryCache()

	assert.Equal(t, want, summariesOf(t, repo, ref))
	assert.Equal(t, 2, *calls)
}
