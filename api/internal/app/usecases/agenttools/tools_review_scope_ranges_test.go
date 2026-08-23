package agenttools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// scopeFixture is one review served by BOTH review tools at once: the file list
// and geometry get_review_scope renders, and the outline post_review_comment
// validates against.
//
// The two come off the same stub for the same reason they come off one
// gitdomain.ReviewScope in production — GetScope loads the geometry through the
// very cache entry GetOutline serves (see branchreview's
// TestGetScope_GeometryMatchesGetOutlineAgainstRealGit). A fixture that fed them
// from two independent fields would let the surface's whole point — that a range
// this tool prints is a range that tool accepts — pass here while being false in
// production.
func scopeFixture(
	t *testing.T,
	base string,
	files []gitdomain.ReviewFileSummary,
	outline []gitdomain.FileOutline,
) *postFixture {
	t.Helper()
	f := postOn(t, "ws-a", outline)
	f.review.base = base
	f.review.files = files
	return f
}

func (f *postFixture) scope(args string) (string, error) {
	return f.ts.Call(context.Background(), "get_review_scope", json.RawMessage(args))
}

// twoHunkScope is the review the anchor tests already measure against, seen from
// the scope side: one modified file whose two hunks put the sides out of step
// (right 12-20 and 42-48, left 12-18 and 40-45), plus a pure addition, so the
// rendering is exercised on a file that has both sides and one that has only the
// right.
func twoHunkScope() ([]gitdomain.ReviewFileSummary, []gitdomain.FileOutline) {
	files := []gitdomain.ReviewFileSummary{
		{Path: "src/auth.go", Status: gitdomain.GitFileStatusModified, Additions: 11, Deletions: 5},
		{Path: "src/new.go", Status: gitdomain.GitFileStatusAdded, Additions: 40},
	}
	outline := append(twoHunkFile(), gitdomain.FileOutline{
		Path:  "src/new.go",
		Hunks: []gitdomain.HunkShape{{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 40}},
	})
	return files, outline
}

// TestGetReviewScope_ReportsTheChangedLineRangesOfEachFile is the gap this
// closes: before it, the tool a model calls before reviewing reported a status,
// two counts and a path, and the ONLY thing on the surface that ever named a line
// number was the rejection it got for guessing one.
func TestGetReviewScope_ReportsTheChangedLineRangesOfEachFile(t *testing.T) {
	files, outline := twoHunkScope()
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(`{}`)
	require.NoError(t, err)
	t.Logf("SCOPE:\n%s", out)

	require.Contains(t, out, "right 12-20, 42-48",
		"the branch side's ranges are what almost every finding anchors in")
	require.Contains(t, out, "left 12-18, 40-45",
		"the base side's numbering diverges from the branch's by every insertion above it; "+
			"a model commenting on removed code with right-side numbers can land in a different hunk")
	require.Contains(t, out, "right 1-40")
	require.NotContains(t, out, "left 0-",
		"a pure addition has no left side, and a zero-span range is not one anybody can anchor to")
}

// A file the review lists but the diff has no lines for — a binary, or an
// untracked file, which the status merges into the file list and `git diff` never
// reports — has to say so. Rendered as a bare row it reads as a file whose ranges
// were merely not printed, and a model would spend a call being refused.
func TestGetReviewScope_MarksFilesNoCommentCanBeAnchoredTo(t *testing.T) {
	files := []gitdomain.ReviewFileSummary{
		{Path: "assets/logo.png", Status: gitdomain.GitFileStatusModified},
		{Path: "notes.txt", Status: gitdomain.GitFileStatusUntracked},
	}
	outline := []gitdomain.FileOutline{{Path: "assets/logo.png", IsBinary: true}}
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(`{}`)
	require.NoError(t, err)
	t.Logf("SCOPE:\n%s", out)

	require.Equal(t, 2, strings.Count(out, "cannot carry an anchored comment"),
		"both the binary file and the untracked one are unanchorable, and for the model's "+
			"purposes they are the same answer")
	// And the claim is true: the tool that would store the comment refuses it.
	_, err = f.post(`{"filePath":"notes.txt","startLine":1,"endLine":1,"side":"right","body":"x"}`)
	require.Error(t, err)
}

// scopeSideRanges matches one side's group on a rendered range line: the label
// and the comma-separated ranges after it. It cannot match the legend, which
// names both sides but carries no line numbers.
var scopeSideRanges = regexp.MustCompile(`(right|left) ((?:\d+-\d+)(?:, \d+-\d+)*)`)

// scopeAnchor is one anchor a model could read off get_review_scope's output and
// hand straight to post_review_comment: a file, a side and a line range.
type scopeAnchor struct {
	path  string
	side  string
	start int
	end   int
}

// anchorsFromScope reads every anchor the rendered scope offered, the way a model
// reads it: an unindented file row names the file, and the indented line under it
// carries that file's ranges per side.
//
// It parses the RENDERED TEXT rather than the fixture deliberately. The property
// under test is that what the tool PRINTS is legal — a test that fed the
// fixture's own hunks back into the validator would prove only that the fixture
// agrees with itself, which it does by construction and which no model ever sees.
func anchorsFromScope(
	t *testing.T,
	out string,
) []scopeAnchor {
	t.Helper()
	var found []scopeAnchor
	path := ""
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "    ") {
			// A file row ends in its path. The fixtures' paths carry no spaces,
			// which is what lets the last field be the path.
			if fields := strings.Fields(line); len(fields) == 4 && strings.HasPrefix(fields[1], "+") {
				path = fields[3]
			}
			continue
		}
		for _, group := range scopeSideRanges.FindAllStringSubmatch(line, -1) {
			found = append(found, parseScopeRanges(t, path, group[1], group[2])...)
		}
	}
	return found
}

func parseScopeRanges(
	t *testing.T,
	path string,
	side string,
	group string,
) []scopeAnchor {
	t.Helper()
	out := make([]scopeAnchor, 0, 2)
	for _, r := range strings.Split(group, ", ") {
		lo, hi, ok := strings.Cut(r, "-")
		require.True(t, ok, "%q is not a range", r)
		start, err := strconv.Atoi(lo)
		require.NoError(t, err)
		end, err := strconv.Atoi(hi)
		require.NoError(t, err)
		out = append(out, scopeAnchor{path: path, side: side, start: start, end: end})
	}
	return out
}

// TestGetReviewScope_EveryRangeItReportsIsAnAnchorPostReviewCommentAccepts is the
// invariant the whole change exists for: the rejection becomes UNREACHABLE for a
// model that read the scope, because the ranges the scope printed are exactly the
// ranges the validator accepts.
//
// The two agree by construction — one outline, resolved once, feeding both the
// listing and the check — and this is what proves the construction holds end to
// end through the rendering, which is the one place it could still be lost: a
// renderer that printed the wrong side's numbers, or an off-by-one on an
// inclusive bound, would break the property while both halves still read the same
// data.
//
// It is the same shape as TestPostReviewComment_TheRejectionsAdviceIsActuallyLegal,
// which holds it on the recovery path. This holds it on the FIRST attempt.
func TestGetReviewScope_EveryRangeItReportsIsAnAnchorPostReviewCommentAccepts(t *testing.T) {
	files, outline := twoHunkScope()
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(`{}`)
	require.NoError(t, err)

	anchors := anchorsFromScope(t, out)
	require.Len(t, anchors, 5,
		"two hunks on both sides of src/auth.go plus one right-side range on src/new.go; "+
			"a parse that found fewer would make every assertion below vacuous")

	for _, a := range anchors {
		retry := scopeFixture(t, "abc123", files, outline)
		_, err := retry.post(fmt.Sprintf(
			`{"filePath":%q,"startLine":%d,"endLine":%d,"side":%q,"body":"x"}`,
			a.path, a.start, a.end, a.side,
		))
		require.NoError(t, err,
			"get_review_scope offered %s %d-%d on %s, which post_review_comment then refused",
			a.side, a.start, a.end, a.path)
		require.Len(t, retry.writer.opens, 1)
	}
}

// The listing is only worth something if it distinguishes: a renderer that
// declared the whole file changed would pass the test above and be useless.
// The unchanged gap between the two hunks must still be refused.
func TestGetReviewScope_TheLinesItDoesNotReportAreStillRefused(t *testing.T) {
	files, outline := twoHunkScope()
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(`{}`)
	require.NoError(t, err)
	require.NotContains(t, out, "21-41", "the gap between the hunks is unchanged context")

	_, err = f.post(anchorArgs(30, 30, "right"))
	require.Error(t, err, "a line the scope did not list must not be anchorable")
	require.Empty(t, f.writer.opens)
}

// manyHunkScopeFile is one file with more changed ranges than a row may print.
func manyHunkScopeFile(path string, hunks int) gitdomain.FileOutline {
	out := gitdomain.FileOutline{Path: path}
	for i := 1; i <= hunks; i++ {
		out.Hunks = append(out.Hunks, gitdomain.HunkShape{
			OldStart: 10 * i, OldLines: 3, NewStart: 10 * i, NewLines: 3,
		})
	}
	return out
}

// One file's ranges are capped per side, and the row says how many it left out —
// a short list read as the whole list is how a model concludes the line it wanted
// is not in the diff.
func TestGetReviewScope_CapsTheRangesOfOneFileAndSaysSo(t *testing.T) {
	over := agenttools.MaxScopeRangesPerFileForTest + 7
	files := []gitdomain.ReviewFileSummary{
		{Path: "src/big.go", Status: gitdomain.GitFileStatusModified, Additions: 90, Deletions: 90},
	}
	f := scopeFixture(t, "abc123", files,
		[]gitdomain.FileOutline{manyHunkScopeFile("src/big.go", over)})

	out, err := f.scope(`{}`)
	require.NoError(t, err)
	t.Logf("SCOPE:\n%s", out)

	shown := agenttools.MaxScopeRangesPerFileForTest
	require.Contains(t, out, fmt.Sprintf("right %s (+%d more)",
		rangeList(1, shown), over-shown))
	require.NotContains(t, out, fmt.Sprintf("%d-%d", 10*(shown+1), 10*(shown+1)+2),
		"the range past the cap must not be printed")
}

// rangeList renders the fixture's own ranges the way the tool does, from the
// first to the nth, so the expectation is spelled out rather than copied.
func rangeList(from, to int) string {
	parts := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		parts = append(parts, fmt.Sprintf("%d-%d", 10*i, 10*i+2))
	}
	return strings.Join(parts, ", ")
}

// A review of many hunk-heavy files must not spend the whole reply on geometry.
// The page has its own budget, and — like every other cap here — it says which
// files it stopped at and names the offset that fetches theirs.
func TestGetReviewScope_CapsTheRangesOfAWholePageAndNamesTheOffsetForTheRest(t *testing.T) {
	// Each file contributes maxScopeRangesPerFile ranges on each side, so the
	// budget runs out well inside a page of 60 files.
	const fileCount = 60
	perFile := 2 * agenttools.MaxScopeRangesPerFileForTest
	var files []gitdomain.ReviewFileSummary
	var outline []gitdomain.FileOutline
	for i := 1; i <= fileCount; i++ {
		path := fmt.Sprintf("src/f%d.go", i)
		files = append(files, gitdomain.ReviewFileSummary{
			Path: path, Status: gitdomain.GitFileStatusModified, Additions: 9, Deletions: 9,
		})
		outline = append(outline, manyHunkScopeFile(path, agenttools.MaxScopeRangesPerFileForTest))
	}
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(`{}`)
	require.NoError(t, err)

	affordable := agenttools.MaxScopeRangesForTest / perFile
	require.Contains(t, out, fmt.Sprintf(
		"Changed line ranges are listed for the first %d files below; the last %d have none listed — "+
			"call get_review_scope with offset=%d for theirs.",
		affordable, fileCount-affordable, affordable,
	))
	require.Equal(t, fileCount, fileRows(out), "every file on the page is still listed")

	// The recovery move works: the offset the note named starts a page whose
	// budget begins again, so those files do get their ranges.
	next, err := f.scope(fmt.Sprintf(`{"offset":%d}`, affordable))
	require.NoError(t, err)
	require.Contains(t, next, fmt.Sprintf("src/f%d.go", affordable+1))
	require.Contains(t, next, "right 10-12",
		"the file the previous page could not afford ranges for must have them here")
}

// The budget is spent on ranges, not on files: a page full of files nothing can
// be anchored to must not exhaust it, or one binary-heavy review would silence
// the geometry of the files that do have some.
func TestGetReviewScope_UnanchorableFilesCostNoRangeBudget(t *testing.T) {
	var files []gitdomain.ReviewFileSummary
	var outline []gitdomain.FileOutline
	for i := 1; i <= 200; i++ {
		path := fmt.Sprintf("assets/a%d.png", i)
		files = append(files, gitdomain.ReviewFileSummary{Path: path, Status: gitdomain.GitFileStatusModified})
		outline = append(outline, gitdomain.FileOutline{Path: path, IsBinary: true})
	}
	files = append(files, gitdomain.ReviewFileSummary{
		Path: "src/auth.go", Status: gitdomain.GitFileStatusModified, Additions: 11, Deletions: 5,
	})
	outline = append(outline, twoHunkFile()...)
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(fmt.Sprintf(`{"limit":%d}`, len(files)))
	require.NoError(t, err)

	require.Contains(t, out, "right 12-20, 42-48",
		"the one anchorable file on this page must still get its ranges")
	require.NotContains(t, out, "Changed line ranges are listed for the first")
}
