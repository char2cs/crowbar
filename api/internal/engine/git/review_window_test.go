package git_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// The four entrypoints of the windowed review API — the surface that replaced
// the whole-diff composite. Each takes the shared read lock and delegates to
// internal/diff; the integration suite covers them through HTTP, which is
// build-tagged and therefore invisible to the unit coverage gate.
//
// One fixture serves all four: a base commit on main, a branch with a committed
// change, an uncommitted edit of a tracked file, and a binary file — so the
// blend (committed + uncommitted) and the binary special-case are both live.
func reviewFixture(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	makeCommit(t, dir, "base.go", "package main\n\nfunc base() {}\n", "base commit")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir,
		"committed.go",
		"package main\n\nfunc committed() { needle() }\n",
		"committed change")
	// git detects binary content by NUL bytes, and emits no @@ headers for it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "logo.bin"), []byte{0, 1, 2, 0, 3}, 0o644))
	gitRun(t, dir, "add", "logo.bin")
	gitRun(t, dir, "commit", "-m", "binary file")

	// Uncommitted edit of a tracked file, so the blend has something to blend.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "base.go"),
		[]byte("package main\n\nfunc base() { needle() }\n"),
		0o644))
	return dir
}

func TestReviewFiles_BlendsCommittedAndUncommitted(t *testing.T) {
	dir := reviewFixture(t)

	summaries, err := git.New().ReviewFiles(context.Background(), dir, "main", nil)
	require.NoError(t, err)

	byPath := map[string]gitdomain.ReviewFileSummary{}
	for _, s := range summaries {
		byPath[s.Path] = s
	}
	require.Contains(t, byPath, "committed.go", "a committed branch change is in the review set")
	require.Contains(t, byPath, "base.go", "so is an uncommitted working-tree edit")
	assert.Positive(t, byPath["committed.go"].Additions)
	assert.Positive(t, byPath["base.go"].Additions,
		"the working-tree edit contributes counts of its own")
	// `Uncommitted` is deliberately NOT set here: the engine reports the blended
	// picture, and branch_review_files.go annotates which half a path came from
	// using the caller's git status. Asserting it at this layer would pin the
	// flag to the wrong owner.
	assert.False(t, byPath["base.go"].Uncommitted)

	// Binary files carry -1 counts, mirroring git numstat's "-" convention, so a
	// real 0/0 text change stays distinguishable from a binary one.
	require.Contains(t, byPath, "logo.bin")
	assert.Equal(t, -1, byPath["logo.bin"].Additions)
	assert.Equal(t, -1, byPath["logo.bin"].Deletions)
}

// The `dirty` argument is the caller's git-status set: it keeps the +/- counts
// off the O(diff size) path by re-diffing only those paths against the working
// tree. Passing it must not change WHAT is reported, only how it is computed.
func TestReviewFiles_DirtyHintMatchesRecompute(t *testing.T) {
	dir := reviewFixture(t)
	e := git.New()
	ctx := context.Background()

	recomputed, err := e.ReviewFiles(ctx, dir, "main", nil)
	require.NoError(t, err)
	hinted, err := e.ReviewFiles(ctx, dir, "main", []string{"base.go"})
	require.NoError(t, err)

	assert.Equal(t, recomputed, hinted)
}

func TestReviewOutline_HunkGeometryAndBinaries(t *testing.T) {
	dir := reviewFixture(t)

	outlines, err := git.New().ReviewOutline(context.Background(), dir, "main")
	require.NoError(t, err)

	byPath := map[string]gitdomain.FileOutline{}
	for _, o := range outlines {
		byPath[o.Path] = o
	}
	require.Contains(t, byPath, "committed.go")
	assert.NotEmpty(t, byPath["committed.go"].Hunks, "a text file is described by its @@ shapes")
	assert.False(t, byPath["committed.go"].IsBinary)

	require.Contains(t, byPath, "logo.bin")
	assert.True(t, byPath["logo.bin"].IsBinary)
	assert.Empty(t, byPath["logo.bin"].Hunks, "git emits no @@ headers for a binary file")
}

func TestReviewFilePatch_WritesOneFilesPatch(t *testing.T) {
	dir := reviewFixture(t)

	var buf bytes.Buffer
	lines, truncated, err := git.New().
		ReviewFilePatch(context.Background(), dir, "main", "committed.go", 0, &buf)
	require.NoError(t, err)

	assert.False(t, truncated, "an unlimited patch of a three-line file is never cut")
	assert.Positive(t, lines)
	assert.Contains(t, buf.String(), "func committed()")
	assert.NotContains(t, buf.String(), "func base()",
		"the patch is scoped to the requested path, not the whole branch")
}

// maxLines cuts on a HUNK boundary, so a cut patch is still a patch — never a
// half-hunk the client cannot render.
func TestReviewFilePatch_TruncatesAtMaxLines(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "wide.txt", "seed\n", "base commit")
	gitRun(t, dir, "checkout", "-b", "feature")

	var body strings.Builder
	for i := range 400 {
		body.WriteString("line ")
		body.WriteString(string(rune('a' + i%26)))
		body.WriteString("\n")
	}
	makeCommit(t, dir, "wide.txt", body.String(), "many lines")

	var buf bytes.Buffer
	_, truncated, err := git.New().
		ReviewFilePatch(context.Background(), dir, "main", "wide.txt", 10, &buf)
	require.NoError(t, err)

	assert.True(t, truncated, "a 400-line change cannot fit in a 10-line cap")
}

func TestReviewSearch_FindsHitsAndReportsTruncation(t *testing.T) {
	dir := reviewFixture(t)
	e := git.New()
	ctx := context.Background()

	hits, truncated, err := e.ReviewSearch(ctx, dir, "main", "needle", gitdomain.SearchOpts{})
	require.NoError(t, err)
	assert.False(t, truncated)
	require.NotEmpty(t, hits, "the token appears in both the committed and the uncommitted change")
	for _, h := range hits {
		assert.Contains(t, h.Preview, "needle")
		assert.Positive(t, h.LineNumber)
		assert.Contains(t, []string{gitdomain.SearchSideOld, gitdomain.SearchSideNew}, h.Side)
	}

	// Limit is what keeps a broad query over a huge diff from being an OOM: the
	// scan stops the moment it fills, and says so.
	limited, truncated, err := e.ReviewSearch(ctx, dir, "main", "needle", gitdomain.SearchOpts{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, limited, 1)
	assert.True(t, truncated)
}

// A find-as-you-type box submits every half-finished pattern the user passes
// through, so an invalid regex must be an error, never a panic.
func TestReviewSearch_InvalidRegexIsAnError(t *testing.T) {
	dir := reviewFixture(t)

	_, _, err := git.New().
		ReviewSearch(context.Background(), dir, "main", "func(", gitdomain.SearchOpts{Regex: true})

	require.Error(t, err)
}

func TestReviewSearch_NoMatchIsEmptyNotAnError(t *testing.T) {
	dir := reviewFixture(t)

	hits, truncated, err := git.New().
		ReviewSearch(context.Background(), dir, "main", "zzz-no-such-token", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.False(t, truncated)
}
