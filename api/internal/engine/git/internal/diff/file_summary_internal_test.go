package diff

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// fsInitRepo creates a fresh, committed git repo for the unexported-function
// tests below. It is a local, minimal stand-in for the diff_test package's own
// initRepo/writeFile/commitAll helpers, which live in package diff_test and so
// are not visible from this white-box (package diff) test file.
func fsInitRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	fsGitRun(t, dir, "init", "-b", "main")
	fsGitRun(t, dir, "config", "user.email", "test@test.com")
	fsGitRun(t, dir, "config", "user.name", "Test User")
	return dir
}

func fsGitRun(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func fsWriteAndCommit(
	t *testing.T,
	dir string,
	name string,
	content string,
	message string,
) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	fsGitRun(t, dir, "add", name)
	fsGitRun(t, dir, "commit", "-m", message)
}

// The byte fixtures below are the verbatim NUL-delimited output of
// `git diff --name-status -M -z` and `git diff --numstat -M -z` captured from a
// real repo, so these tests pin the parser against git's actual wire format
// (single-char status + path; R<score> + old + new for renames; a trailing
// empty path field that flags a rename in numstat; "-" counts for binary).

func TestParseNameStatusZ(t *testing.T) {
	in := "A\x00added.txt\x00" +
		"M\x00keep.txt\x00" +
		"R100\x00old.txt\x00new.txt\x00" +
		"D\x00gone.txt\x00"

	got := parseNameStatusZ(in)

	require.Len(t, got, 4)
	assert.Equal(t, gitdomain.ReviewFileSummary{Path: "added.txt", Status: gitdomain.GitFileStatusAdded}, got[0])
	assert.Equal(t, gitdomain.ReviewFileSummary{Path: "keep.txt", Status: gitdomain.GitFileStatusModified}, got[1])
	assert.Equal(t, gitdomain.ReviewFileSummary{
		Path:    "new.txt",
		OldPath: "old.txt",
		Status:  gitdomain.GitFileStatusRenamed,
	}, got[2])
	assert.Equal(t, gitdomain.ReviewFileSummary{Path: "gone.txt", Status: gitdomain.GitFileStatusDeleted}, got[3])
}

func TestParseNumstatZ_TextRenameAndBinary(t *testing.T) {
	in := "3\t1\tkeep.txt\x00" +
		"0\t0\t\x00old.txt\x00new.txt\x00" +
		"-\t-\tlogo.png\x00"

	got := parseNumstatZ(in)

	require.Len(t, got, 3)
	assert.Equal(t, numCount{additions: 3, deletions: 1}, got["keep.txt"])
	// A rename's counts are keyed by the NEW path, never the old one.
	assert.Equal(t, numCount{additions: 0, deletions: 0}, got["new.txt"])
	_, hasOld := got["old.txt"]
	assert.False(t, hasOld, "rename counts must not be keyed by the source path")
	// Binary files carry the numstat "-" as -1 so a real 0/0 stays distinct.
	assert.Equal(t, numCount{additions: -1, deletions: -1}, got["logo.png"])
}

func TestStatusFromCode(t *testing.T) {
	assert.Equal(t, gitdomain.GitFileStatusAdded, statusFromCode("A"))
	assert.Equal(t, gitdomain.GitFileStatusModified, statusFromCode("M"))
	assert.Equal(t, gitdomain.GitFileStatusDeleted, statusFromCode("D"))
	assert.Equal(t, gitdomain.GitFileStatusRenamed, statusFromCode("R100"))
	assert.Equal(t, gitdomain.GitFileStatusRenamed, statusFromCode("C75"))
	assert.Equal(t, gitdomain.GitFileStatusModified, statusFromCode("T"))
	assert.Equal(t, gitdomain.GitFileStatusModified, statusFromCode(""))
}

func TestParseCount(t *testing.T) {
	assert.Equal(t, 7, parseCount("7"))
	assert.Equal(t, 0, parseCount("0"))
	assert.Equal(t, -1, parseCount("-"), "binary marker maps to -1")
	assert.Equal(t, 0, parseCount("garbage"), "unparseable count falls back to 0")
}

// ---------------------------------------------------------------------------
// worktreeCounts / treeCounts / headCommit / pathspecCounts: exec.Git failure
// branches, none of which are reachable through FileSummaries — a ref that is
// valid enough for the name-status call FileSummaries runs first is valid for
// these too, so the only way to exercise their own RequireSuccess checks is to
// call them directly against a directory that is not a git repo at all.
// ---------------------------------------------------------------------------

func TestWorktreeCounts_NonRepoDir_ReturnsError(t *testing.T) {
	_, err := worktreeCounts(context.Background(), t.TempDir(), "HEAD")
	require.Error(t, err)
}

func TestTreeCounts_NonRepoDir_ReturnsError(t *testing.T) {
	_, err := treeCounts(context.Background(), t.TempDir(), "HEAD", "deadbeef")
	require.Error(t, err)
}

func TestPathspecCounts_NonRepoDir_ReturnsError(t *testing.T) {
	stale := []gitdomain.ReviewFileSummary{{Path: "a.txt", Status: gitdomain.GitFileStatusModified}}
	_, err := pathspecCounts(context.Background(), t.TempDir(), "HEAD", stale)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// headCommit
// ---------------------------------------------------------------------------

func TestHeadCommit_NoCommits_ReturnsFalse(t *testing.T) {
	dir := fsInitRepo(t) // git init only — HEAD is unborn, rev-parse HEAD fails.
	sha, ok := headCommit(context.Background(), dir)
	assert.False(t, ok)
	assert.Empty(t, sha)
}

func TestHeadCommit_WithCommit_ReturnsSHATrue(t *testing.T) {
	dir := fsInitRepo(t)
	fsWriteAndCommit(t, dir, "a.txt", "1\n", "c1")
	sha, ok := headCommit(context.Background(), dir)
	assert.True(t, ok)
	assert.Len(t, sha, 40, "rev-parse HEAD must yield a full SHA")
}

// ---------------------------------------------------------------------------
// summaryCounts: the two early-exit branches that skip the committed/dirty
// split entirely.
// ---------------------------------------------------------------------------

// TestSummaryCounts_RangeRef_UsesWorktreeCountsDirectly pins the reasoning in
// isRangeRef's doc comment: a ref naming both ends of a diff cannot be split,
// because the committed half appends HEAD to it, and git refuses a three-commit
// diff outright. A range ref must be routed straight to the unsplit query and
// produce exactly what that query alone would.
func TestSummaryCounts_RangeRef_UsesWorktreeCountsDirectly(t *testing.T) {
	dir := fsInitRepo(t)
	fsWriteAndCommit(t, dir, "a.txt", "1\n", "c1")
	fsWriteAndCommit(t, dir, "a.txt", "1\n2\n", "c2")
	ctx := context.Background()
	entries := []gitdomain.ReviewFileSummary{{Path: "a.txt", Status: gitdomain.GitFileStatusModified}}

	got, err := summaryCounts(ctx, dir, "HEAD~1..HEAD", entries, []string{})
	require.NoError(t, err)

	want, err := worktreeCounts(ctx, dir, "HEAD~1..HEAD")
	require.NoError(t, err)
	assert.Equal(t, want, got, "a range ref must produce exactly what the unsplit query alone produces")
	assert.Equal(t, numCount{additions: 1, deletions: 0}, got["a.txt"])
}

// TestSummaryCounts_HeadCommitFails_FallsBackWithoutQueryingTreeCounts covers
// the guard for an unborn HEAD (a repo with zero commits): summaryCounts must
// fall back to the unsplit worktreeCounts query and must NOT attempt the
// ref->HEAD cached query first — proven here by failing the test outright if
// the committed-count seam is ever invoked.
func TestSummaryCounts_HeadCommitFails_FallsBackWithoutQueryingTreeCounts(t *testing.T) {
	dir := fsInitRepo(t) // no commits at all: HEAD is unborn.
	prevRunGit := runGit
	t.Cleanup(func() { runGit = prevRunGit })
	runGit = func(_ context.Context, _ string, _ ...string) exec.Result {
		t.Fatal("treeCounts must not run once headCommit has already failed")
		return exec.Result{}
	}

	// worktreeCounts itself also fails against an unborn HEAD (no baseline to
	// diff against) — what matters is which branch was taken to get there.
	_, err := summaryCounts(context.Background(), dir, "HEAD", []gitdomain.ReviewFileSummary{}, []string{})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// mergeCounts
// ---------------------------------------------------------------------------

// TestMergeCounts_StaleEntryMissingFromFresh_DroppedFromResult covers the case
// a file was flagged stale (dirty), then reverted back to its committed content
// before the re-diff ran: the fresh re-fetch finds no change for it at all, and
// the merge must not fall back to the now-stale committed count for that path.
func TestMergeCounts_StaleEntryMissingFromFresh_DroppedFromResult(t *testing.T) {
	entries := []gitdomain.ReviewFileSummary{
		{Path: "a.txt", Status: gitdomain.GitFileStatusModified},
		{Path: "b.txt", Status: gitdomain.GitFileStatusModified},
	}
	committed := map[string]numCount{
		"a.txt": {additions: 5, deletions: 2},
		"b.txt": {additions: 1, deletions: 1},
	}
	stale := []gitdomain.ReviewFileSummary{{Path: "a.txt", Status: gitdomain.GitFileStatusModified}}
	fresh := map[string]numCount{}

	got := mergeCounts(entries, committed, stale, fresh)

	assert.NotContains(t, got, "a.txt",
		"a stale entry absent from the fresh re-diff must not fall back to the stale committed count")
	assert.Equal(t, numCount{additions: 1, deletions: 1}, got["b.txt"],
		"an entry never marked stale keeps its committed count untouched")
}

// ---------------------------------------------------------------------------
// numstatEntry: malformed token-stream guards. Real `git diff --numstat -z`
// never emits these shapes, but the parser must not panic or misindex if it
// ever sees one.
// ---------------------------------------------------------------------------

func TestNumstatEntry_TooFewFields_SkipsToken(t *testing.T) {
	path, count, next := numstatEntry([]string{"no-tabs-here"}, 0)
	assert.Empty(t, path)
	assert.Equal(t, numCount{}, count)
	assert.Equal(t, 1, next)
}

// TestNumstatEntry_RenameShapeTruncated_ReturnsEmptyPath covers a rename-shaped
// entry ("added\tdeleted\t", meant to be followed by \x00-delimited old/new path
// tokens) whose token stream is truncated right after the empty third field,
// missing the two path tokens it promised.
func TestNumstatEntry_RenameShapeTruncated_ReturnsEmptyPath(t *testing.T) {
	path, count, next := numstatEntry([]string{"3\t1\t"}, 0)
	assert.Empty(t, path)
	assert.Equal(t, numCount{additions: 3, deletions: 1}, count)
	assert.Equal(t, 1, next)
}
