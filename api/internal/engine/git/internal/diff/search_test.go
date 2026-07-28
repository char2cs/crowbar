package diff_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// searchFixture builds the repo every line-number assertion in this file is
// measured against. Its shape is deliberate: alpha.txt carries two changes far
// enough apart to produce two separate hunks, and src/beta.txt carries an
// INSERTION before a later change, so from that point on the new-side numbers
// no longer equal the old-side ones. A parser that reported the old number, or
// that counted lines from the top of the file rather than from each @@ header,
// passes on alpha.txt and fails on beta.txt.
//
// The resulting diff (verified against git):
//
//	@@ -2,7 +2,7 @@ alpha 1        @@ -8,6 +8,7 @@ beta 7
//	@@ -22,7 +22,7 @@ alpha 21      @@ -17,7 +18,7 @@ beta 16
func searchFixture(
	t *testing.T,
) (string, string) {
	t.Helper()
	dir := initRepo(t)
	writeFile(t, dir, "alpha.txt", searchAlphaText(t, "alpha 5 REMOVEME", "alpha 25"))
	writeFile(t, dir, "src/beta.txt", searchBetaText(t, false, "beta 20"))
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	writeFile(t, dir, "alpha.txt", searchAlphaText(t, "alpha 5 ADDED", "alpha 25 ADDED"))
	writeFile(t, dir, "src/beta.txt", searchBetaText(t, true, "beta 20 CHANGED"))
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "branch work")
	return dir, base
}

// searchAlphaText renders 30 lines "alpha 1".."alpha 30" with lines 5 and 25
// replaced by the given content.
func searchAlphaText(
	t *testing.T,
	line5 string,
	line25 string,
) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		b.WriteString(searchAlphaLine(i, line5, line25))
		b.WriteString("\n")
	}
	return b.String()
}

func searchAlphaLine(
	i int,
	line5 string,
	line25 string,
) string {
	if i == 5 {
		return line5
	}
	if i == 25 {
		return line25
	}
	return fmt.Sprintf("alpha %d", i)
}

// searchBetaText renders 25 lines "beta 1".."beta 25" with line 20 replaced by
// line20, optionally inserting an extra line after line 10 — the insertion that
// pushes every later new-side number one past its old-side number.
func searchBetaText(
	t *testing.T,
	inserted bool,
	line20 string,
) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 25; i++ {
		b.WriteString(searchBetaLine(i, line20))
		b.WriteString("\n")
		if i == 10 && inserted {
			b.WriteString("beta INSERTED\n")
		}
	}
	return b.String()
}

func searchBetaLine(
	i int,
	line20 string,
) string {
	if i == 20 {
		return line20
	}
	return fmt.Sprintf("beta %d", i)
}

// TestSearchDiff_AttributesSidesAndLineNumbers is the core assertion of this
// file: a removed line reports the OLD number, a context line reports the NEW
// number, and both stay correct in a second file whose numbering has been
// shifted by an earlier insertion. " beta 19" sits at old line 19 and new line
// 20; reporting 19 would send the reader to the wrong line.
func TestSearchDiff_AttributesSidesAndLineNumbers(t *testing.T) {
	dir, base := searchFixture(t)

	hits, truncated, err := diff.SearchDiff(
		context.Background(), dir, base, "REMOVEME|alpha 8|beta 19",
		gitdomain.SearchOpts{Regex: true},
	)

	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "alpha.txt", Side: "old", LineNumber: 5, Preview: "alpha 5 REMOVEME"},
		{Path: "alpha.txt", Side: "new", LineNumber: 8, Preview: "alpha 8"},
		{Path: "src/beta.txt", Side: "new", LineNumber: 20, Preview: "beta 19"},
	}, hits)
}

// TestSearchDiff_OrdersAddedLinesAcrossHunksAndFiles pins the new-side counter
// across two hunks of one file and two hunks of another. "beta 20 CHANGED" is
// new line 21 rather than 20 because of the insertion ten lines above it.
func TestSearchDiff_OrdersAddedLinesAcrossHunksAndFiles(t *testing.T) {
	dir, base := searchFixture(t)

	hits, truncated, err := diff.SearchDiff(
		context.Background(), dir, base, "ADDED|INSERTED|CHANGED",
		gitdomain.SearchOpts{Regex: true},
	)

	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "alpha.txt", Side: "new", LineNumber: 5, Preview: "alpha 5 ADDED"},
		{Path: "alpha.txt", Side: "new", LineNumber: 25, Preview: "alpha 25 ADDED"},
		{Path: "src/beta.txt", Side: "new", LineNumber: 11, Preview: "beta INSERTED"},
		{Path: "src/beta.txt", Side: "new", LineNumber: 21, Preview: "beta 20 CHANGED"},
	}, hits)
}

// TestSearchDiff_ReportsBothSidesOfAChangedLine: the removal and the
// replacement both match "beta 20", and they are reported in stream order with
// their own side and number.
func TestSearchDiff_ReportsBothSidesOfAChangedLine(t *testing.T) {
	dir, base := searchFixture(t)

	hits, _, err := diff.SearchDiff(
		context.Background(), dir, base, "beta 20", gitdomain.SearchOpts{},
	)

	require.NoError(t, err)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "src/beta.txt", Side: "old", LineNumber: 20, Preview: "beta 20"},
		{Path: "src/beta.txt", Side: "new", LineNumber: 21, Preview: "beta 20 CHANGED"},
	}, hits)
}

// TestSearchDiff_SkipsDiffMachineryLines: the patch's own scaffolding is not
// content. "alpha 21" is the funcname git appends to the second hunk header
// (`@@ -22,7 +22,7 @@ alpha 21`) and line 21 is in no hunk body, so a hit for it
// could only have come from the header.
func TestSearchDiff_SkipsDiffMachineryLines(t *testing.T) {
	dir, base := searchFixture(t)

	for _, query := range []string{"diff --git", "@@ -22", "index ", "alpha.txt", "src/beta.txt", "alpha 21"} {
		t.Run(query, func(t *testing.T) {
			hits, truncated, err := diff.SearchDiff(
				context.Background(), dir, base, query, gitdomain.SearchOpts{},
			)
			require.NoError(t, err)
			assert.False(t, truncated)
			assert.Empty(t, hits)
		})
	}
}

// TestSearchDiff_LimitTruncates: the cap is what keeps the response bounded on
// a million-line diff, so it must cut at exactly Limit and report it — and the
// hits it keeps must be the first ones, not an arbitrary subset.
func TestSearchDiff_LimitTruncates(t *testing.T) {
	dir, base := searchFixture(t)
	all, truncated, err := diff.SearchDiff(
		context.Background(), dir, base, "alpha", gitdomain.SearchOpts{},
	)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Greater(t, len(all), 2)

	hits, truncated, err := diff.SearchDiff(
		context.Background(), dir, base, "alpha", gitdomain.SearchOpts{Limit: 2},
	)

	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Equal(t, all[:2], hits)
}

// TestSearchDiff_LiteralQueryIsNotAPattern: without Regex the query is matched
// verbatim, so a user searching for "alpha [0-9]+ ADDED" finds nothing rather
// than two lines.
func TestSearchDiff_LiteralQueryIsNotAPattern(t *testing.T) {
	dir, base := searchFixture(t)
	const pattern = `alpha [0-9]+ ADDED`

	literal, _, err := diff.SearchDiff(context.Background(), dir, base, pattern, gitdomain.SearchOpts{})
	require.NoError(t, err)
	assert.Empty(t, literal)

	matched, _, err := diff.SearchDiff(
		context.Background(), dir, base, pattern, gitdomain.SearchOpts{Regex: true},
	)
	require.NoError(t, err)
	require.Len(t, matched, 2)
	assert.Equal(t, 5, matched[0].LineNumber)
	assert.Equal(t, 25, matched[1].LineNumber)
}

func TestSearchDiff_CaseSensitivity(t *testing.T) {
	dir, base := searchFixture(t)

	insensitive, _, err := diff.SearchDiff(
		context.Background(), dir, base, "removeme", gitdomain.SearchOpts{},
	)
	require.NoError(t, err)
	require.Len(t, insensitive, 1)
	assert.Equal(t, "old", insensitive[0].Side)
	assert.Equal(t, 5, insensitive[0].LineNumber)

	sensitive, _, err := diff.SearchDiff(
		context.Background(), dir, base, "removeme", gitdomain.SearchOpts{CaseSensitive: true},
	)
	require.NoError(t, err)
	assert.Empty(t, sensitive)
}

// TestSearchDiff_InvalidRegexIsAnError: a half-typed pattern arrives on every
// keystroke of a find-in-diff box. It must come back as an error, never a panic.
func TestSearchDiff_InvalidRegexIsAnError(t *testing.T) {
	dir, base := searchFixture(t)

	require.NotPanics(t, func() {
		hits, truncated, err := diff.SearchDiff(
			context.Background(), dir, base, "alpha (", gitdomain.SearchOpts{Regex: true},
		)
		require.Error(t, err)
		assert.Empty(t, hits)
		assert.False(t, truncated)
	})
}

func TestSearchDiff_NoMatchesIsNotAnError(t *testing.T) {
	dir, base := searchFixture(t)

	hits, truncated, err := diff.SearchDiff(
		context.Background(), dir, base, "nothing-here-at-all", gitdomain.SearchOpts{},
	)

	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Empty(t, hits)
}

func TestSearchDiff_EmptyQueryMatchesNothing(t *testing.T) {
	dir, base := searchFixture(t)

	hits, truncated, err := diff.SearchDiff(context.Background(), dir, base, "", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Empty(t, hits)
}

func TestSearchDiff_UnknownRefIsAnError(t *testing.T) {
	dir, _ := searchFixture(t)

	_, _, err := diff.SearchDiff(
		context.Background(), dir, "no-such-ref-anywhere", "alpha", gitdomain.SearchOpts{},
	)

	require.Error(t, err)
}

// TestSearchDiff_LongLineBeyondScannerLimit: bufio.Scanner's default 64 KB
// token limit would abort the whole scan on a minified bundle, and a preview of
// the whole line would put 70 KB into a JSON response per hit.
func TestSearchDiff_LongLineBeyondScannerLimit(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "kept\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	long := strings.Repeat("var a=1;", 9000) + "NEEDLE"
	require.Greater(t, len(long), 64*1024)
	writeFile(t, dir, "bundle.min.js", long+"\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "minified")

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "NEEDLE", gitdomain.SearchOpts{})

	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "bundle.min.js", hits[0].Path)
	assert.Equal(t, "new", hits[0].Side)
	assert.Equal(t, 1, hits[0].LineNumber)
	assert.Len(t, hits[0].Preview, 200)
	assert.Equal(t, long[:200], hits[0].Preview)
}

// TestSearchDiff_PreviewCutsOnARuneBoundary: 200 bytes lands inside a 3-byte
// rune here, and a preview cut there is invalid UTF-8 and serialises as U+FFFD.
func TestSearchDiff_PreviewCutsOnARuneBoundary(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "kept\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	wide := strings.Repeat("日", 300)
	writeFile(t, dir, "wide.txt", wide+"\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "wide")

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "日", gitdomain.SearchOpts{})

	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.True(t, utf8.ValidString(hits[0].Preview))
	assert.LessOrEqual(t, len(hits[0].Preview), 200)
	assert.Equal(t, wide[:198], hits[0].Preview)
}

// TestSearchDiff_AddedFileNumbersFromOne: an added file's hunk header is
// `@@ -0,0 +1,N @@`, so the new-side counter must start from the header's own
// value rather than from any file-level assumption.
func TestSearchDiff_AddedFileNumbersFromOne(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "kept\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	writeFile(t, dir, "fresh.txt", "one\ntwo FIND\nthree\nfour FIND\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "add")

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "FIND", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "fresh.txt", Side: "new", LineNumber: 2, Preview: "two FIND"},
		{Path: "fresh.txt", Side: "new", LineNumber: 4, Preview: "four FIND"},
	}, hits)
}

// TestSearchDiff_DeletedFileKeepsItsPath: a deletion's new-side header is
// `+++ /dev/null`, so the path has to come from the old side or every hit in a
// deleted file is unattributable.
func TestSearchDiff_DeletedFileKeepsItsPath(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "kept\n")
	writeFile(t, dir, "src/doomed.txt", "one\ntwo FIND\nthree\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	mustGit(t, dir, "rm", "-q", "src/doomed.txt")
	mustGit(t, dir, "commit", "-m", "delete")

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "FIND", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "src/doomed.txt", Side: "old", LineNumber: 2, Preview: "two FIND"},
	}, hits)
}

// TestSearchDiff_AwkwardPaths: a path reaches the hit exactly as it is on disk,
// which takes undoing three separate things git does to the name in a patch
// header. A name with a space is followed by a TAB field separator; a name with
// a quote is C-quoted whole; and a non-ASCII name is octal-escaped unless
// core.quotePath is turned off. Any of the three left in place yields a path
// the client cannot open.
func TestSearchDiff_AwkwardPaths(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "keep.txt", "kept\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	writeFile(t, dir, "with space.txt", "one FIND\n")
	writeFile(t, dir, "src/wärme.txt", "two FIND\n")
	writeFile(t, dir, `quote"name.txt`, "three FIND\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "awkward")

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "FIND", gitdomain.SearchOpts{})

	require.NoError(t, err)
	require.Len(t, hits, 3)
	var paths []string
	for _, hit := range hits {
		paths = append(paths, hit.Path)
		assert.Equal(t, 1, hit.LineNumber)
	}
	assert.ElementsMatch(
		t,
		[]string{"with space.txt", "src/wärme.txt", `quote"name.txt`},
		paths,
	)
}

// TestSearchDiff_IncludesUncommittedEdits: the search must cover the same diff
// the review shows — committed work since ref PLUS working-tree edits — or a
// user searches a patch they are not looking at.
func TestSearchDiff_IncludesUncommittedEdits(t *testing.T) {
	dir, base := searchFixture(t)
	writeFile(t, dir, "alpha.txt", searchAlphaText(t, "alpha 5 ADDED", "alpha 25 UNSAVED"))

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "UNSAVED", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "alpha.txt", Side: "new", LineNumber: 25, Preview: "alpha 25 UNSAVED"},
	}, hits)
}

// TestSearchDiff_BinaryFileYieldsNoContentHits: git emits "Binary files … differ"
// in place of a hunk body, which is machinery, not content.
func TestSearchDiff_BinaryFileYieldsNoContentHits(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "logo.bin", "\x00\x01\x02\x03")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)

	writeFile(t, dir, "logo.bin", "\x00\x09\x08\x07")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "change")

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "Binary", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.Empty(t, hits)
}
