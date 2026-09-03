package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestGetReviewScope_CapsTheChangedFileListAndSaysSo(t *testing.T) {
	over := tools.DefaultScopeFilesForTest + 50
	stub := &stubReviewReader{base: "abc123", files: manyFiles(over)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Equal(t, tools.DefaultScopeFilesForTest, fileRows(out))
	require.Contains(t, out, fmt.Sprintf(
		"Showing changed files 1-%d of %d. 50 not shown — call get_review_scope with offset=%d for the next page.",
		tools.DefaultScopeFilesForTest, over, tools.DefaultScopeFilesForTest,
	))
	// The base ref survives the cap: it is the anchor for every finding, and a
	// paged file list that lost it would describe a diff against nothing.
	require.Contains(t, out, "abc123")
}

// A capped file list that could not be paged past would turn a large diff into a
// review of its first hundred files — post_review_comment rejects a path that is
// not in the review, so an unreachable file is an unwritable finding.
func TestGetReviewScope_PagesPastTheCap(t *testing.T) {
	over := tools.DefaultScopeFilesForTest + 50
	stub := &stubReviewReader{base: "abc123", files: manyFiles(over)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope",
		json.RawMessage(fmt.Sprintf(`{"offset":%d}`, tools.DefaultScopeFilesForTest)))
	require.NoError(t, err)

	require.Equal(t, 50, fileRows(out))
	require.Contains(t, out, fmt.Sprintf("src/f%d.go", over))
	require.Contains(t, out, "This is the last page.")
}

func TestGetReviewScope_ClampsAnOversizedLimit(t *testing.T) {
	stub := &stubReviewReader{base: "abc123", files: manyFiles(tools.MaxScopeFilesForTest + 40)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{"limit":99999}`))
	require.NoError(t, err)

	require.Equal(t, tools.MaxScopeFilesForTest, fileRows(out))
}

func TestGetReviewScope_ANegativeOffsetReadsAsTheFirstPage(t *testing.T) {
	review := &stubReviewReader{base: "abc123", files: manyFiles(3)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, review)

	out, err := ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{"offset":-1}`))
	require.NoError(t, err)

	require.Contains(t, out, "Showing all 3 changed files.")
	require.Equal(t, 3, fileRows(out))
	require.Contains(t, out, "src/f1.go")
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

// One file's ranges are capped per side, and the row says how many it left out —
// a short list read as the whole list is how a model concludes the line it wanted
// is not in the diff.
func TestGetReviewScope_CapsTheRangesOfOneFileAndSaysSo(t *testing.T) {
	over := tools.MaxScopeRangesPerFileForTest + 7
	files := []gitdomain.ReviewFileSummary{
		{Path: "src/big.go", Status: gitdomain.GitFileStatusModified, Additions: 90, Deletions: 90},
	}
	f := scopeFixture(t, "abc123", files,
		[]gitdomain.FileOutline{manyHunkScopeFile("src/big.go", over)})

	out, err := f.scope(`{}`)
	require.NoError(t, err)
	t.Logf("SCOPE:\n%s", out)

	shown := tools.MaxScopeRangesPerFileForTest
	require.Contains(t, out, fmt.Sprintf("right %s (+%d more)",
		rangeList(1, shown), over-shown))
	require.NotContains(t, out, fmt.Sprintf("%d-%d", 10*(shown+1), 10*(shown+1)+2),
		"the range past the cap must not be printed")
}

// A review of many hunk-heavy files must not spend the whole reply on geometry.
// The page has its own budget, and — like every other cap here — it says which
// files it stopped at and names the offset that fetches theirs.
func TestGetReviewScope_CapsTheRangesOfAWholePageAndNamesTheOffsetForTheRest(t *testing.T) {
	// Each file contributes maxScopeRangesPerFile ranges on each side, so the
	// budget runs out well inside a page of 60 files.
	const fileCount = 60
	perFile := 2 * tools.MaxScopeRangesPerFileForTest
	var files []gitdomain.ReviewFileSummary
	var outline []gitdomain.FileOutline
	for i := 1; i <= fileCount; i++ {
		path := fmt.Sprintf("src/f%d.go", i)
		files = append(files, gitdomain.ReviewFileSummary{
			Path: path, Status: gitdomain.GitFileStatusModified, Additions: 9, Deletions: 9,
		})
		outline = append(outline, manyHunkScopeFile(path, tools.MaxScopeRangesPerFileForTest))
	}
	f := scopeFixture(t, "abc123", files, outline)

	out, err := f.scope(`{}`)
	require.NoError(t, err)

	affordable := tools.MaxScopeRangesForTest / perFile
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

func TestGetReviewScope_ReportsBaseAndChangedFiles(t *testing.T) {
	stub := &stubReviewReader{
		base: "abc123def",
		files: []gitdomain.ReviewFileSummary{
			{Path: "src/auth.go", Status: gitdomain.GitFileStatusModified, Additions: 10, Deletions: 3},
			{Path: "src/new.go", Status: gitdomain.GitFileStatusAdded, Additions: 40, Deletions: 0},
		},
	}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, out, "abc123def")
	require.Contains(t, out, "src/auth.go")
	require.Contains(t, out, "+10")
	require.Contains(t, out, "-3")
	require.Contains(t, out, "src/new.go")
	require.Contains(t, out, "+40")
	require.Contains(t, out, "-0")
	require.Equal(t, 1, stub.scopeCalls,
		"one tool call must resolve the review scope once: the base ref and the file "+
			"list come out of the same resolution, which is several git subprocesses")
}
