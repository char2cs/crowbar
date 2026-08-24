package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

func TestPostReviewComment_AnchorsAndMarksItselfAsAgent(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	out, err := f.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":"This leaks the token."}`,
	)
	require.NoError(t, err)
	require.Contains(t, out, "thread-1")

	require.Len(t, f.writer.opens, 1)
	got := f.writer.opens[0]
	// The caller's OWN workspace, resolved from its runner — never an argument.
	require.Equal(t, "ws-a", got.WsID)
	require.Equal(t, "src/auth.go", got.FilePath)
	require.Equal(t, 42, got.StartLine)
	require.Equal(t, 44, got.EndLine)
	// LineNumber is the aggregate's pre-range anchor; leaving it zero would render
	// the comment against line 0.
	require.Equal(t, 42, got.LineNumber)
	require.Equal(t, domain.ReviewSideRight, got.Side)
	require.Equal(t, "This leaks the token.", got.Body)
	require.True(t, got.IsAgent, "an agent-written finding must be marked as one")
	require.NotEmpty(t, got.Author, "an unattributed comment renders as a blank name")
	require.Equal(t, callerProviderID, got.Author, "the author must come from the caller's runner provider")
	require.NotEmpty(t, got.ID)
	require.NotEmpty(t, got.MessageID)
	require.NotEqual(t, got.ID, got.MessageID)
}

// TestPostReviewComment_ValidatesAgainstTheCallersOwnReview is the scoping property
// for the WRITE tool: the outline an anchor is checked against must be the caller's
// own workspace's diff. Validating against another workspace's geometry would accept
// anchors that float on the diff the comment is actually attached to.
func TestPostReviewComment_ValidatesAgainstTheCallersOwnReview(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"x"}`)
	require.NoError(t, err)
	require.Equal(t, "ws-a", f.review.lastWsID,
		"the outline must be read from the caller's own workspace, never another")

	forbidden := []string{"wsId", "wsID", "workspaceId", "workspace_id"}
	for _, tool := range f.ts.Tools() {
		if tool.Name != "post_review_comment" {
			continue
		}
		for _, bad := range forbidden {
			require.NotContains(t, string(tool.InputSchema), bad,
				"post_review_comment exposes %s; scope must never be an argument", bad)
		}
	}
}

// TestPostReviewComment_BroadcastsSoAnOpenReviewPaneSeesIt is the point of the whole
// tool: the review-thread repository does not fan out, and the agent write bypasses
// the HTTP handler that normally pushes the frame, so without an explicit broadcast a
// posted finding is stored and INVISIBLE until the user remounts the pane.
func TestPostReviewComment_BroadcastsSoAnOpenReviewPaneSeesIt(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak"}`)
	require.NoError(t, err)

	require.Len(t, f.broadcast.frames, 1, "a stored finding must reach connected clients")
	frame := f.broadcast.frames[0]
	require.Equal(t, "thread-1", frame.thread.ID)
	require.Equal(t, "ws-a", frame.thread.WsID)
	// The /threads stream filters on projectId AND repoId as well as wsId, so a frame
	// missing either is delivered to nobody.
	require.Equal(t, "P", frame.projectID)
	require.Equal(t, "R", frame.repoID)
	require.NotEmpty(t, frame.thread.Messages, "the frame must carry the finding itself")
	require.True(t, frame.thread.Messages[0].IsAgent)
}

func TestPostReviewComment_ARejectedAnchorBroadcastsNothing(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":200,"endLine":200,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Empty(t, f.broadcast.frames, "a rejected anchor must not announce a thread that does not exist")
}

// A dedup hit wrote nothing, so it must not announce anything either: a second frame
// would make every client re-render a thread it already has.
func TestPostReviewComment_ARetryBroadcastsOnlyOnce(t *testing.T) {
	args := `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"k"}`
	first := postOn(t, "ws-a", authHunk())
	_, err := first.post(args)
	require.NoError(t, err)

	_, err = first.retryOn(t, "ws-a", authHunk()).post(args)
	require.NoError(t, err)

	require.Len(t, first.writer.opens, 1)
	require.Len(t, first.broadcast.frames, 1, "a retry that wrote nothing must not emit a frame")
}

// The whole correctness risk: an anchor outside any hunk floats off the diff, so
// the user sees a finding with no code beside it. The assertion that matters is
// that NOTHING was written, not that an error came back.
func TestPostReviewComment_RejectsAnAnchorOutsideAnyHunk(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":200,"endLine":200,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get_review_scope")
	require.Empty(t, f.writer.opens, "a rejected anchor must never reach the thread store")
}

// TestPostReviewComment_AnchorEdgesAreInclusive pins the off-by-one directly. The
// fixture hunk starts at 40 and spans 10 lines, so it covers 40..49 INCLUSIVE:
// widening either comparison by one accepts a line that renders outside the hunk.
func TestPostReviewComment_AnchorEdgesAreInclusive(t *testing.T) {
	cases := []struct {
		name     string
		line     int
		accepted bool
	}{
		{"one line before the hunk", 39, false},
		{"the hunk's first line", 40, true},
		{"the hunk's last line", 49, true},
		{"one line past the hunk", 50, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := postOn(t, "ws-a", authHunk())
			_, err := f.post(fmt.Sprintf(
				`{"filePath":"src/auth.go","startLine":%d,"endLine":%d,"side":"right","body":"x"}`,
				tc.line, tc.line,
			))
			if !tc.accepted {
				require.Error(t, err, "line %d is outside the 40-49 hunk", tc.line)
				require.Empty(t, f.writer.opens)
				return
			}
			require.NoError(t, err, "line %d is inside the 40-49 hunk", tc.line)
			require.Len(t, f.writer.opens, 1)
		})
	}
}

// The same two edges for a RANGE: a range may touch both ends of the hunk but not
// cross either.
func TestPostReviewComment_AnchorRangeEdgesAreInclusive(t *testing.T) {
	cases := []struct {
		name       string
		start, end int
		accepted   bool
	}{
		{"exactly the hunk", 40, 49, true},
		{"starts one line early", 39, 49, false},
		{"ends one line late", 40, 50, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := postOn(t, "ws-a", authHunk())
			_, err := f.post(fmt.Sprintf(
				`{"filePath":"src/auth.go","startLine":%d,"endLine":%d,"side":"right","body":"x"}`,
				tc.start, tc.end,
			))
			if !tc.accepted {
				require.Error(t, err)
				require.Empty(t, f.writer.opens)
				return
			}
			require.NoError(t, err)
			require.Len(t, f.writer.opens, 1)
		})
	}
}

// A range that starts inside the hunk but runs past its end is still floating for
// most of its length, so it is rejected whole rather than silently clamped.
func TestPostReviewComment_RejectsARangeThatOverrunsTheHunk(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":48,"endLine":60,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Empty(t, f.writer.opens)
}

func TestPostReviewComment_RejectsAnUnknownFile(t *testing.T) {
	f := postOn(t, "ws-a", outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 1, OldLines: 100, NewStart: 1, NewLines: 100,
	}))

	_, err := f.post(`{"filePath":"src/untouched.go","startLine":5,"endLine":5,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "src/untouched.go")
	require.Contains(t, err.Error(), "get_review_scope")
	require.Empty(t, f.writer.opens, "a file outside the review must never reach the thread store")
	require.Empty(t, f.broadcast.frames)
}

// The two numberings diverge by every insertion above them. The hunk here makes
// line 12 valid on the LEFT and invalid on the RIGHT, and line 105 the reverse, so
// an implementation that read the wrong pair of the hunk's four numbers fails both
// halves of this test.
func TestPostReviewComment_LeftSideAnchorsAgainstOldLineNumbers(t *testing.T) {
	skewed := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 10, OldLines: 5, NewStart: 100, NewLines: 20,
	})
	f := postOn(t, "ws-a", skewed)

	_, err := f.post(`{"filePath":"src/auth.go","startLine":12,"endLine":12,"side":"left","body":"removed too much"}`)
	require.NoError(t, err, "line 12 is inside the hunk's OLD range")
	require.Len(t, f.writer.opens, 1)
	require.Equal(t, domain.ReviewSideLeft, f.writer.opens[0].Side)

	_, err = f.post(`{"filePath":"src/auth.go","startLine":12,"endLine":12,"side":"right","body":"wrong side"}`)
	require.Error(t, err, "line 12 is outside the hunk's NEW range")
	require.Len(t, f.writer.opens, 1, "the rejected post must not have been written")

	_, err = f.post(`{"filePath":"src/auth.go","startLine":105,"endLine":105,"side":"left","body":"wrong side"}`)
	require.Error(t, err, "line 105 is outside the hunk's OLD range")
	require.Len(t, f.writer.opens, 1)

	_, err = f.post(`{"filePath":"src/auth.go","startLine":105,"endLine":105,"side":"right","body":"added badly"}`)
	require.NoError(t, err, "line 105 is inside the hunk's NEW range")
	require.Len(t, f.writer.opens, 2)
	require.Equal(t, domain.ReviewSideRight, f.writer.opens[1].Side)
}

// The left side's edges come off a DIFFERENT pair of numbers, so they need their own
// boundary case: this hunk covers 10..14 on the left.
func TestPostReviewComment_LeftSideEdgesAreInclusive(t *testing.T) {
	skewed := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 10, OldLines: 5, NewStart: 100, NewLines: 20,
	})
	for line, accepted := range map[int]bool{9: false, 10: true, 14: true, 15: false} {
		f := postOn(t, "ws-a", skewed)
		_, err := f.post(fmt.Sprintf(
			`{"filePath":"src/auth.go","startLine":%d,"endLine":%d,"side":"left","body":"x"}`, line, line,
		))
		if !accepted {
			require.Error(t, err, "left line %d is outside the 10-14 old range", line)
			require.Empty(t, f.writer.opens)
			continue
		}
		require.NoError(t, err, "left line %d is inside the 10-14 old range", line)
		require.Len(t, f.writer.opens, 1)
	}
}

// A rename is addressed by either of its names, so a model that read the file
// under its old path can still anchor to it.
func TestPostReviewComment_AcceptsARenamedFileUnderEitherPath(t *testing.T) {
	f := postOn(t, "ws-a", []gitdomain.FileOutline{{
		Path:    "src/new_name.go",
		OldPath: "src/old_name.go",
		Hunks:   []gitdomain.HunkShape{{OldStart: 1, OldLines: 20, NewStart: 1, NewLines: 20}},
	}})

	_, err := f.post(`{"filePath":"src/old_name.go","startLine":3,"endLine":3,"side":"right","body":"here"}`)
	require.NoError(t, err)
	require.Len(t, f.writer.opens, 1)
}

// A binary file has no hunks at all, so there is no line for a comment to sit on.
func TestPostReviewComment_RejectsABinaryFile(t *testing.T) {
	f := postOn(t, "ws-a", []gitdomain.FileOutline{{Path: "assets/logo.png", IsBinary: true}})

	_, err := f.post(`{"filePath":"assets/logo.png","startLine":1,"endLine":1,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Empty(t, f.writer.opens)
}

// A hunk whose side is ABSENT (`@@ -1,2 +0,0 @@` gives that side start 0, span 0)
// covers no lines, so nothing may anchor to it.
func TestPostReviewComment_RejectsAnAnchorOnAnAbsentSide(t *testing.T) {
	f := postOn(t, "ws-a", outlineWithHunk("src/gone.go", gitdomain.HunkShape{
		OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0,
	}))

	_, err := f.post(`{"filePath":"src/gone.go","startLine":1,"endLine":1,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Empty(t, f.writer.opens)
}

// The retry a dropped MCP response produces arrives on a NEW ToolSet, which is why
// the dedup map is a dependency rather than ToolSet state.
func TestPostReviewComment_IdempotencyKeyCollapsesARetry(t *testing.T) {
	args := `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"leak-in-auth"}`
	first := postOn(t, "ws-a", authHunk())
	out1, err := first.post(args)
	require.NoError(t, err)

	out2, err := first.retryOn(t, "ws-a", authHunk()).post(args)
	require.NoError(t, err)

	require.Len(t, first.writer.opens, 1, "a retry with the same key must open exactly one thread")
	require.Contains(t, out1, "thread-1")
	require.Contains(t, out2, "thread-1", "the retry must report the thread the first call opened")
}

// A retry that reuses a key but moves the lines wrote nothing, so it must be
// answered with the anchor that IS stored — echoing its own arguments would report a
// comment at a location no thread is anchored to.
func TestPostReviewComment_ARetryReportsTheStoredAnchorNotItsArguments(t *testing.T) {
	first := postOn(t, "ws-a", authHunk())
	_, err := first.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"k"}`,
	)
	require.NoError(t, err)

	out, err := first.retryOn(t, "ws-a", authHunk()).post(
		`{"filePath":"src/auth.go","startLine":47,"endLine":48,"side":"right","body":"leak","idempotencyKey":"k"}`,
	)
	require.NoError(t, err)

	require.Len(t, first.writer.opens, 1)
	require.Contains(t, out, "42-42", "the reply must describe the thread that exists")
	require.NotContains(t, out, "47-48", "reporting the unstored arguments would be a lie")
}

func TestPostReviewComment_DifferentKeysOpenDifferentThreads(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"one","idempotencyKey":"finding-a"}`,
	)
	require.NoError(t, err)
	_, err = f.post(
		`{"filePath":"src/auth.go","startLine":43,"endLine":43,"side":"right","body":"two","idempotencyKey":"finding-b"}`,
	)
	require.NoError(t, err)

	require.Len(t, f.writer.opens, 2)
	require.Len(t, f.broadcast.frames, 2)
}

// Keys are scoped by workspace: two agents reviewing two branches will invent the
// same obvious key, and the second finding must not be swallowed as a retry of the
// first. ws-a and ws-a1 are sibling review surfaces in the fixture tree.
func TestPostReviewComment_SameKeyInTwoWorkspacesDoesNotCollide(t *testing.T) {
	args := `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"leak-in-auth"}`
	onA := postOn(t, "ws-a", authHunk())
	_, err := onA.post(args)
	require.NoError(t, err)

	_, err = onA.retryOn(t, "ws-a1", authHunk()).post(args)
	require.NoError(t, err)

	require.Len(t, onA.writer.opens, 2, "the same key in a different workspace is a different finding")
	require.Equal(t, "ws-a", onA.writer.opens[0].WsID)
	require.Equal(t, "ws-a1", onA.writer.opens[1].WsID)
}

// No key means no dedup: a model that omits it gets a thread per call.
func TestPostReviewComment_WithoutAKeyEveryCallOpensAThread(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())
	args := `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak"}`

	_, err := f.post(args)
	require.NoError(t, err)
	_, err = f.post(args)
	require.NoError(t, err)

	require.Len(t, f.writer.opens, 2)
	require.Len(t, f.broadcast.frames, 2)
}

// A failed write must not be remembered as done, or the retry the key exists for
// would return success having stored nothing.
func TestPostReviewComment_AFailedWriteIsNotRemembered(t *testing.T) {
	args := `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"k"}`
	f := postOn(t, "ws-a", authHunk())
	f.writer.err = errNotFoundForTest

	_, err := f.post(args)
	require.Error(t, err)
	require.Empty(t, f.broadcast.frames, "nothing was stored, so nothing may be announced")

	f.writer.err = nil
	out, err := f.retryOn(t, "ws-a", authHunk()).post(args)
	require.NoError(t, err)
	require.Contains(t, out, "thread-1")
	require.Len(t, f.writer.opens, 1)
	require.Len(t, f.broadcast.frames, 1)
}

// The store error must not be double-prefixed with the package name on its way to
// the model.
func TestPostReviewComment_ErrorNamesThePackageOnce(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())
	f.writer.err = errNotFoundForTest

	_, err := f.post(`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak"}`)
	require.Error(t, err)
	require.Equal(t, 1, strings.Count(err.Error(), "agenttools:"), "got %q", err.Error())
}

func TestPostReviewComment_RejectsBadArguments(t *testing.T) {
	cases := map[string]string{
		"unknown side":  `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"middle","body":"x"}`,
		"missing side":  `{"filePath":"src/auth.go","startLine":42,"endLine":42,"body":"x"}`,
		"blank body":    `{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"   "}`,
		"zero start":    `{"filePath":"src/auth.go","startLine":0,"endLine":42,"side":"right","body":"x"}`,
		"end before":    `{"filePath":"src/auth.go","startLine":44,"endLine":42,"side":"right","body":"x"}`,
		"not an object": `[]`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			f := postOn(t, "ws-a", authHunk())
			_, err := f.post(args)
			require.Error(t, err)
			require.Empty(t, f.writer.opens)
			require.Empty(t, f.broadcast.frames)
		})
	}
}

// post_review_comment is the first WRITE tool, so its fail-closed wiring is worth
// asserting directly: with no outline reader it cannot validate an anchor, with no
// dedup map a retry would duplicate a finding, and with no broadcaster the finding
// never reaches the pane the user is watching. Any of those and it must not exist.
func TestPostReviewComment_NotAdvertisedWithoutItsDependencies(t *testing.T) {
	full := func() tools.Deps {
		return tools.Deps{
			Review:          &stubReviewReader{outline: authHunk()},
			ThreadWrites:    &stubThreadWriter{},
			Idempotency:     tools.NewIdempotency(),
			ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
		}
	}
	cases := map[string]func(*tools.Deps){
		"no review reader": func(d *tools.Deps) { d.Review = nil },
		"no thread writer": func(d *tools.Deps) { d.ThreadWrites = nil },
		"no dedup map":     func(d *tools.Deps) { d.Idempotency = nil },
		"no broadcaster":   func(d *tools.Deps) { d.ThreadBroadcast = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			deps := full()
			drop(&deps)
			m, err := tools.NewTokenMinter()
			require.NoError(t, err)
			deps.Resolver = tools.NewResolver(m,
				stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
				stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
				stubWorkspaces{all: tree()})
			ts := tools.NewToolSet(deps, "RUN", m.Mint("RUN"))
			for _, tool := range ts.Tools() {
				require.NotEqual(t, "post_review_comment", tool.Name)
			}
			_, err = ts.Call(context.Background(), "post_review_comment", json.RawMessage(`{}`))
			require.Error(t, err)
		})
	}
}

// TestPostReviewComment_CarriesTheCallersProviderAndChat is the attribution
// property for the open path: the review UI can only say WHICH agent left a
// finding, and offer a way back to the conversation it came out of, if both ids
// are stamped on the message at write time — nothing downstream can recover them
// later.
func TestPostReviewComment_CarriesTheCallersProviderAndChat(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":"This leaks the token."}`,
	)
	require.NoError(t, err)

	require.Len(t, f.writer.opens, 1)
	got := f.writer.opens[0]
	require.Equal(t, callerProviderID, got.ProviderID,
		"the provider must come from the caller's runner, never from an argument")
	require.Equal(t, "CHAT", got.ChatID,
		"the chat must be the runner's CURRENT chat, so the finding links back to where it was reasoned")
}

func TestPostReviewComment_RefusesAnOverlongBodyAndNamesTheLimit(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())
	over := tools.MaxWrittenBodyCharsForTest + 1

	_, err := f.post(fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":%q}`,
		longBody(over),
	))

	require.Error(t, err)
	// The model's only recovery is to shorten and retry, which it cannot do
	// against an error that merely says "too long".
	require.Contains(t, err.Error(), fmt.Sprint(over))
	require.Contains(t, err.Error(), fmt.Sprint(tools.MaxWrittenBodyCharsForTest))
	require.Empty(t, f.writer.opens, "a refused body must never reach the store")
	require.Empty(t, f.broadcast.frames)
}

func TestPostReviewComment_AcceptsABodyExactlyAtTheLimit(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":%q}`,
		longBody(tools.MaxWrittenBodyCharsForTest),
	))

	require.NoError(t, err, "the limit is inclusive")
	require.Len(t, f.writer.opens, 1)
}

// RUNES, not bytes — the same rule truncateBody counts by. A body of exactly the
// limit in multi-byte characters is three times the limit in bytes and must
// still be accepted, or the cap silently becomes three times stricter for
// anything that is not ASCII.
func TestPostReviewComment_CountsTheBodyInRunesNotBytes(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())
	body := strings.Repeat("é", tools.MaxWrittenBodyCharsForTest)
	require.Greater(t, len(body), tools.MaxWrittenBodyCharsForTest, "the fixture must be multi-byte")

	_, err := f.post(fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":%q}`, body,
	))

	require.NoError(t, err)
	require.Len(t, f.writer.opens, 1)
}
