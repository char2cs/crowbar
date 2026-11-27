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

func TestSearchDiff_NonexistentRepoPath_ReturnsStartError(t *testing.T) {
	_, _, err := diff.SearchDiff(
		context.Background(), "/nonexistent/path/xyz123", "HEAD", "alpha", gitdomain.SearchOpts{},
	)

	require.Error(t, err)
}

func TestSearchDiff_BlankContextLine(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "f.txt", "a\nb\n\nc\nd\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)
	writeFile(t, dir, "f.txt", "a\nb\n\nc\nMATCHME\n")

	hits, truncated, err := diff.SearchDiff(context.Background(), dir, base, "MATCHME", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "f.txt", Side: "new", LineNumber: 5, Preview: "MATCHME"},
	}, hits)
}

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

func TestSearchDiff_IncludesUncommittedEdits(t *testing.T) {
	dir, base := searchFixture(t)
	writeFile(t, dir, "alpha.txt", searchAlphaText(t, "alpha 5 ADDED", "alpha 25 UNSAVED"))

	hits, _, err := diff.SearchDiff(context.Background(), dir, base, "UNSAVED", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "alpha.txt", Side: "new", LineNumber: 25, Preview: "alpha 25 UNSAVED"},
	}, hits)
}

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
