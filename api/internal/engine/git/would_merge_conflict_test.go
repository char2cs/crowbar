package git_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// oneVerdict asks the batch whether merging theirs into main would conflict.
func oneVerdict(ctx context.Context, dir, theirs string) git.MergeVerdict {
	return git.New().WouldMergeConflicts(ctx, dir, []git.MergePair{{Ours: "main", Theirs: theirs}})[0]
}

func TestWouldMergeConflicts_NonOverlappingIsClean(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.txt", "feature\n", "feature work")

	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "main.txt", "main\n", "main work")

	got := oneVerdict(ctx, dir, "feature")
	require.NoError(t, got.Err)
	assert.False(t, got.Conflict, "edits to different files merge cleanly")
}

func TestWouldMergeConflicts_OverlappingEditsConflict(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "shared.txt", "base\n", "base")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "shared.txt", "feature change\n", "feature edit")

	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "shared.txt", "main change\n", "main edit")

	got := oneVerdict(ctx, dir, "feature")
	require.NoError(t, got.Err)
	assert.True(t, got.Conflict, "diverging edits to the same line conflict")
}

// Critical regression: git merge-tree exits 1 for a conflict AND for an
// unresolvable ref, but the latter writes no tree to stdout. The unresolvable
// case must return an ERROR (so the caller fails OPEN and allows the merge),
// never be misread as a conflict that wrongly blocks a clean branch.
func TestWouldMergeConflicts_UnknownRefErrors(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")

	got := oneVerdict(ctx, dir, "no-such-branch")
	require.Error(t, got.Err)
	assert.False(t, got.Conflict, "an unresolvable ref must not be reported as a conflict")
}

// Critical regression: a missing worktree directory must error (fail open), not
// be misread as a conflict.
func TestWouldMergeConflicts_MissingRepoErrors(t *testing.T) {
	ctx := context.Background()
	got := oneVerdict(ctx, "/no/such/repo/path/xyz123", "feature")
	require.Error(t, got.Err)
	assert.False(t, got.Conflict, "a missing worktree must not be reported as a conflict")
}

// The batch answers each pair — clean, conflicting, and unresolvable (an error,
// so the caller fails open) — in input order.
func TestWouldMergeConflicts_AnswersEachPairInOrder(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "shared.txt", "base\n", "base")
	gitRun(t, dir, "checkout", "-b", "clean")
	makeCommit(t, dir, "clean.txt", "x\n", "clean work")
	gitRun(t, dir, "checkout", "-b", "clash", "main")
	makeCommit(t, dir, "shared.txt", "clash\n", "clash edit")
	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "shared.txt", "main\n", "main edit")

	pairs := []git.MergePair{
		{Ours: "main", Theirs: "clean"},
		{Ours: "main", Theirs: "clash"},
		{Ours: "main", Theirs: "no-such-branch"},
	}
	got := git.New().WouldMergeConflicts(ctx, dir, pairs)

	require.Len(t, got, len(pairs))
	require.NoError(t, got[0].Err)
	assert.False(t, got[0].Conflict)
	require.NoError(t, got[1].Err)
	assert.True(t, got[1].Conflict)
	require.Error(t, got[2].Err, "an unresolvable branch fails open, never reads as a conflict")
	assert.False(t, got[2].Conflict)
}

// A verdict is a fact about two commits, so moving a branch must change the
// answer on the very next call: nothing may be remembered by branch NAME.
func TestWouldMergeConflicts_FollowsABranchThatMoves(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "shared.txt", "base\n", "base")
	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.txt", "x\n", "feature work")
	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "shared.txt", "main\n", "main edit")
	e := git.New()
	pairs := []git.MergePair{{Ours: "main", Theirs: "feature"}}

	require.False(t, e.WouldMergeConflicts(ctx, dir, pairs)[0].Conflict)

	gitRun(t, dir, "checkout", "feature")
	makeCommit(t, dir, "shared.txt", "feature\n", "feature edits the same line")
	gitRun(t, dir, "checkout", "main")

	assert.True(t, e.WouldMergeConflicts(ctx, dir, pairs)[0].Conflict)
}

// agedRefs backdates every ref file and directory, as a repo whose branches have
// not moved for a while looks. Refs touched within the last moment are never
// trusted by the tip memory (git's own racy-timestamp rule), so a test that
// wants it engaged has to say they are old.
func agedRefs(tb testing.TB, dir string) {
	tb.Helper()
	old := time.Now().Add(-time.Hour)
	require.NoError(tb, filepath.WalkDir(filepath.Join(dir, ".git", "refs"), func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, old, old)
	}))
}

// The branch tips are read once while no ref moves: a repeated list costs no git
// process at all, and a ref that moves is seen on the very next read.
func TestWouldMergeConflicts_ReadsTheTipsOnlyWhenARefMoved(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "base.txt", "base\n", "base")
	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.txt", "x\n", "feature work")
	gitRun(t, dir, "checkout", "main")
	agedRefs(t, dir)

	var mu sync.Mutex
	runs := map[string]int{}
	e := git.NewWithExec(func(ctx context.Context, d string, args ...string) gitexec.Result {
		mu.Lock()
		runs[args[0]]++
		mu.Unlock()
		return gitexec.Git(ctx, d, args...)
	})
	pairs := []git.MergePair{{Ours: "main", Theirs: "feature"}}

	require.False(t, e.WouldMergeConflicts(ctx, dir, pairs)[0].Conflict)
	require.False(t, e.WouldMergeConflicts(ctx, dir, pairs)[0].Conflict)
	require.False(t, e.WouldMergeConflicts(ctx, dir, pairs)[0].Conflict)

	mu.Lock()
	assert.Equal(t, 1, runs["for-each-ref"], "tips are read once while no ref moves")
	assert.Equal(t, 1, runs["merge-tree"], "the verdict is remembered by commit")
	mu.Unlock()

	gitRun(t, dir, "checkout", "feature")
	makeCommit(t, dir, "base.txt", "feature rewrites base\n", "feature edits base")
	gitRun(t, dir, "checkout", "main")
	makeCommit(t, dir, "base.txt", "main rewrites base\n", "main edits base")

	assert.True(t, e.WouldMergeConflicts(ctx, dir, pairs)[0].Conflict, "a moved ref is seen on the next read")
	mu.Lock()
	assert.Equal(t, 2, runs["for-each-ref"])
	mu.Unlock()
}
