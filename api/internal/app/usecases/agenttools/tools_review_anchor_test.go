package agenttools_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// twoHunkFile is a REALISTIC two-hunk diff of one file, which the single-hunk
// fixture every other anchor test uses cannot be: with one hunk there is no
// "between the hunks", no "spans two hunks", and no way to tell "inside a hunk"
// apart from "inside the first hunk".
//
// The numbers are a diff git could actually emit. Hunk 1 turns 7 old lines into 9
// new ones, so everything below it shifts by +2 — which is why hunk 2 starts at
// old 40 and new 42, and why the two sides of this file do not agree about which
// lines are changed.
//
//	right (new): 12..20 and 42..48
//	left  (old): 12..18 and 40..45
//
// The gap between the hunks is unchanged context, and it is wide: git's default is
// 3 context lines, so a function whose body is longer than that has its signature
// outside the hunk covering the body. That is the ordinary case, not a corner one.
func twoHunkFile() []gitdomain.FileOutline {
	return []gitdomain.FileOutline{{
		Path: "src/auth.go",
		Hunks: []gitdomain.HunkShape{
			{OldStart: 12, OldLines: 7, NewStart: 12, NewLines: 9},
			{OldStart: 40, OldLines: 6, NewStart: 42, NewLines: 7},
		},
	}}
}

func anchorArgs(start, end int, side string) string {
	return fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":%d,"endLine":%d,"side":%q,"body":"x"}`,
		start, end, side,
	)
}

// TestPostReviewComment_MultiHunkAnchors is the wall anchorInAnyHunk builds,
// tested against a diff that actually has two hunks.
//
// The rule is that the WHOLE range must sit inside ONE hunk. There is no union
// across hunks anywhere in the codebase, so an anchor that covers both a changed
// range and the unchanged code between them is refused — deliberately, because a
// comment that floats off the diff shows the user a finding with no code beside it.
//
// Each rejected case asserts the message names the LINES and the FILE, so a
// rejection cannot pass this test by being the wrong rejection: "src/auth.go is
// not part of this review" would otherwise satisfy a bare require.Error and hide a
// fixture whose paths never matched at all.
func TestPostReviewComment_MultiHunkAnchors(t *testing.T) {
	cases := []struct {
		name       string
		start, end int
		side       string
		accepted   bool
	}{
		{"inside the first hunk", 13, 14, "right", true},
		{"inside the second hunk", 45, 46, "right", true},
		{"spanning both hunks", 18, 44, "right", false},
		{"a function signature in the context between them", 30, 30, "right", false},
		{"starting in the gap and ending in the second hunk", 38, 44, "right", false},
		{"covering the whole file", 1, 60, "right", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := postOn(t, "ws-a", twoHunkFile())
			out, err := f.post(anchorArgs(tc.start, tc.end, tc.side))
			if tc.accepted {
				require.NoError(t, err)
				require.Len(t, f.writer.opens, 1)
				require.Contains(t, out, "src/auth.go")
				return
			}
			require.Error(t, err)
			t.Logf("REJECTION[%s]: %s", tc.name, err.Error())
			require.Contains(t, err.Error(), fmt.Sprintf("%d-%d", tc.start, tc.end),
				"the rejection must be the ANCHOR rejection, naming the lines it refused")
			require.Contains(t, err.Error(), "src/auth.go")
			// The legal move, spelled out. A model cannot recover from being told
			// only that it was wrong: get_review_scope reports a status, two counts
			// and a path per file, so nothing on the tool surface would tell it
			// where the changed lines are.
			require.Contains(t, err.Error(), "12-20, 42-48",
				"the rejection must name the ranges an anchor may legally sit in")
			require.NotContains(t, err.Error(), "12-18, 40-45",
				"those are the LEFT side's ranges; naming them for a right-side anchor sends the model to lines that do not apply")
			require.Empty(t, f.writer.opens, "a rejected anchor must never reach the thread store")
			require.Empty(t, f.broadcast.frames)
		})
	}
}

// TestPostReviewComment_MultiHunkSideSelectsTheNumbering pins that side chooses
// which pair of each hunk's four numbers is consulted, on a file where the two
// sides genuinely disagree: hunk 1 inserts two lines, so below it the new
// numbering runs two ahead of the old.
//
// Line 41 is inside the old range of hunk 2 (40..45) and in the unchanged gap on
// the new side. Line 47 is the reverse. An implementation that read the wrong pair
// therefore fails in BOTH directions, which is what stops this passing on a
// swapped implementation that happened to be symmetric.
func TestPostReviewComment_MultiHunkSideSelectsTheNumbering(t *testing.T) {
	cases := []struct {
		name     string
		line     int
		side     string
		accepted bool
	}{
		{"41 is in the old range of the second hunk", 41, "left", true},
		{"41 is in the unchanged gap on the new side", 41, "right", false},
		{"47 is in the new range of the second hunk", 47, "right", true},
		{"47 is past the old range of the second hunk", 47, "left", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := postOn(t, "ws-a", twoHunkFile())
			_, err := f.post(anchorArgs(tc.line, tc.line, tc.side))
			if tc.accepted {
				require.NoError(t, err, "line %d must be anchorable on the %s side", tc.line, tc.side)
				require.Len(t, f.writer.opens, 1)
				return
			}
			require.Error(t, err, "line %d must not be anchorable on the %s side", tc.line, tc.side)
			t.Logf("REJECTION[%s]: %s", tc.name, err.Error())
			require.Contains(t, err.Error(), tc.side,
				"the rejection must name the side whose numbering it was measured against")
			require.Empty(t, f.writer.opens)
		})
	}
}

// rejectionRangeList captures the whole list of ranges a rejection offered, read
// back out of the message exactly as a model would read it — up to the first "."
// or "(", which is where the list ends and the caveats or the next sentence begin.
var rejectionRangeList = regexp.MustCompile(`are: ([\d, -]+?)\s*[.(]`)

// offeredRanges is every range a rejection told the model it could re-anchor
// inside.
func offeredRanges(t *testing.T, message string) [][2]int {
	t.Helper()
	found := rejectionRangeList.FindStringSubmatch(message)
	require.Len(t, found, 2, "the rejection offered no ranges at all: %s", message)
	out := make([][2]int, 0, 2)
	for _, r := range strings.Split(found[1], ", ") {
		lo, hi, ok := strings.Cut(r, "-")
		require.True(t, ok, "%q is not a range", r)
		start, err := strconv.Atoi(lo)
		require.NoError(t, err)
		end, err := strconv.Atoi(hi)
		require.NoError(t, err)
		out = append(out, [2]int{start, end})
	}
	return out
}

// TestPostReviewComment_TheRejectionsAdviceIsActuallyLegal is what makes the
// message more than decoration: EVERY range it tells a model to re-anchor inside
// is fed straight back to the tool, and every one has to land.
//
// A message naming ranges the validator would then refuse is worse than the terse
// one it replaced — it looks actionable and costs the model a turn.
//
// Every offered range is retried, not just the first, and that is not
// belt-and-braces: on this fixture the first hunk's OLD range (12-18) happens to
// sit inside its NEW range (12-20), so a hint that read the wrong side would still
// offer a legal first range and a test that stopped there would pass against it.
// The second range is where the two sides diverge.
func TestPostReviewComment_TheRejectionsAdviceIsActuallyLegal(t *testing.T) {
	for _, side := range []string{"right", "left"} {
		t.Run(side, func(t *testing.T) {
			f := postOn(t, "ws-a", twoHunkFile())
			_, err := f.post(anchorArgs(18, 44, side))
			require.Error(t, err)

			offered := offeredRanges(t, err.Error())
			require.Len(t, offered, 2, "this file has two hunks, so it has two legal ranges")
			for _, r := range offered {
				retry := postOn(t, "ws-a", twoHunkFile())
				_, err := retry.post(anchorArgs(r[0], r[1], side))
				require.NoError(t, err,
					"the rejection offered %d-%d on the %s side, which the validator then refused",
					r[0], r[1], side)
				require.Len(t, retry.writer.opens, 1)
			}
		})
	}
}

// manyHunkFile is a file with more changed ranges than a rejection may print:
// thirty hunks, three lines each, ten lines apart. A real file can carry up to a
// thousand (MaxOutlineHunksPerFile), so the list has to be bounded.
func manyHunkFile() []gitdomain.FileOutline {
	hunks := make([]gitdomain.HunkShape, 0, 30)
	for i := 1; i <= 30; i++ {
		hunks = append(hunks, gitdomain.HunkShape{
			OldStart: 10 * i, OldLines: 3, NewStart: 10 * i, NewLines: 3,
		})
	}
	return []gitdomain.FileOutline{{Path: "src/auth.go", Hunks: hunks}}
}

// TestPostReviewComment_ARejectionListsTheRangesNearestTheAnchor pins WHICH ranges
// a bounded list keeps.
//
// Truncating from the top of the file would be the obvious implementation and the
// useless one: a model that anchored at line 205 would be handed the ranges at
// line 10 and left exactly as stuck as before. The window follows the anchor
// instead, and the count of what it left out is stated, because a model reads a
// short list as the whole list.
func TestPostReviewComment_ARejectionListsTheRangesNearestTheAnchor(t *testing.T) {
	f := postOn(t, "ws-a", manyHunkFile())

	_, err := f.post(anchorArgs(205, 205, "right"))
	require.Error(t, err)
	t.Logf("REJECTION[capped list]: %s", err.Error())

	require.Contains(t, err.Error(), "200-202", "the nearest range must be offered")
	require.Contains(t, err.Error(), "160-162", "the window's first range")
	require.Contains(t, err.Error(), "230-232", "the window's last range")
	require.NotContains(t, err.Error(), "150-152", "one range before the window")
	require.NotContains(t, err.Error(), "240-242", "one range past the window")
	require.NotContains(t, err.Error(), "10-12",
		"a list truncated from the top of the file leaves an anchor at line 205 no better off")
	require.Contains(t, err.Error(), "22 further changed ranges",
		"a bounded list that does not say what it left out reads as the whole list")
}

// A file whose hunks ran past the outline's own collection cap has ranges the
// daemon never saw, so the list is a lower bound and must say so — otherwise a
// model reads "the changed ranges are ..." as exhaustive and concludes the line it
// wanted is not in the diff.
func TestPostReviewComment_ARejectionAdmitsAPartialOutline(t *testing.T) {
	partial := twoHunkFile()
	partial[0].IsPartial = true
	f := postOn(t, "ws-a", partial)

	_, err := f.post(anchorArgs(30, 30, "right"))
	require.Error(t, err)
	t.Logf("REJECTION[partial outline]: %s", err.Error())
	require.Contains(t, err.Error(), "more hunks than the review's outline collected")
}

// A deleted file's lines exist only on the left, so an anchor on the right is one
// argument away from success — and the rejection has to say which argument. The
// test then FOLLOWS that advice, so a message pointing at a side that would also
// fail cannot pass.
func TestPostReviewComment_ARejectionOnAnAbsentSidePointsAtTheOtherOne(t *testing.T) {
	deleted := outlineWithHunk("src/gone.go", gitdomain.HunkShape{
		OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0,
	})
	f := postOn(t, "ws-a", deleted)

	_, err := f.post(`{"filePath":"src/gone.go","startLine":1,"endLine":1,"side":"right","body":"x"}`)
	require.Error(t, err)
	t.Logf("REJECTION[absent side]: %s", err.Error())
	require.Contains(t, err.Error(), `side="left"`, "the recovery move is the other side, and it must be named")
	require.Contains(t, err.Error(), "1-2", "the other side's ranges are what makes that move usable")

	follow := postOn(t, "ws-a", deleted)
	_, err = follow.post(`{"filePath":"src/gone.go","startLine":1,"endLine":1,"side":"left","body":"x"}`)
	require.NoError(t, err, "the rejection told the model to use the left side; that must work")
	require.Len(t, follow.writer.opens, 1)
}

// A binary file has no lines on either side, so there is no argument that makes an
// anchor work and the model must be told to stop rather than to try the other
// side. Telling the two no-lines cases apart is the whole point of separating them.
func TestPostReviewComment_ARejectionOnABinaryFileSaysNoAnchorExists(t *testing.T) {
	f := postOn(t, "ws-a", []gitdomain.FileOutline{{Path: "assets/logo.png", IsBinary: true}})

	_, err := f.post(`{"filePath":"assets/logo.png","startLine":1,"endLine":1,"side":"right","body":"x"}`)
	require.Error(t, err)
	t.Logf("REJECTION[binary]: %s", err.Error())
	require.Contains(t, err.Error(), "binary")
	require.Contains(t, err.Error(), "cannot carry an anchored comment")
	require.NotContains(t, err.Error(), `side="left"`,
		"there is no other side to try, and sending a model to one costs it a turn")
	require.Empty(t, f.writer.opens)
}
