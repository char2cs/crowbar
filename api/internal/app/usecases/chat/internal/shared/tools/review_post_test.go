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
			require.Contains(t, err.Error(), "12-20, 42-48",
				"the rejection must name the ranges an anchor may legally sit in")
			require.NotContains(t, err.Error(), "12-18, 40-45",
				"those are the LEFT side's ranges; naming them for a right-side anchor sends the model to lines that do not apply")
			require.Empty(t, f.writer.opens, "a rejected anchor must never reach the thread store")
			require.Empty(t, f.broadcast.frames)
		})
	}
}

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

func TestPostReviewComment_ARejectionAdmitsAPartialOutline(t *testing.T) {
	partial := twoHunkFile()
	partial[0].IsPartial = true
	f := postOn(t, "ws-a", partial)

	_, err := f.post(anchorArgs(30, 30, "right"))
	require.Error(t, err)
	t.Logf("REJECTION[partial outline]: %s", err.Error())
	require.Contains(t, err.Error(), "more hunks than the review's outline collected")
}

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
	require.Equal(t, "ws-a", got.WsID)
	require.Equal(t, "src/auth.go", got.FilePath)
	require.Equal(t, 42, got.StartLine)
	require.Equal(t, 44, got.EndLine)
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

func TestPostReviewComment_BroadcastsSoAnOpenReviewPaneSeesIt(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak"}`)
	require.NoError(t, err)

	require.Len(t, f.broadcast.frames, 1, "a stored finding must reach connected clients")
	frame := f.broadcast.frames[0]
	require.Equal(t, "thread-1", frame.thread.ID)
	require.Equal(t, "ws-a", frame.thread.WsID)
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

func TestPostReviewComment_RejectsAnAnchorOutsideAnyHunk(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(`{"filePath":"src/auth.go","startLine":200,"endLine":200,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get_review_scope")
	require.Empty(t, f.writer.opens, "a rejected anchor must never reach the thread store")
}

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

func TestPostReviewComment_RejectsABinaryFile(t *testing.T) {
	f := postOn(t, "ws-a", []gitdomain.FileOutline{{Path: "assets/logo.png", IsBinary: true}})

	_, err := f.post(`{"filePath":"assets/logo.png","startLine":1,"endLine":1,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Empty(t, f.writer.opens)
}

func TestPostReviewComment_RejectsAnAnchorOnAnAbsentSide(t *testing.T) {
	f := postOn(t, "ws-a", outlineWithHunk("src/gone.go", gitdomain.HunkShape{
		OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0,
	}))

	_, err := f.post(`{"filePath":"src/gone.go","startLine":1,"endLine":1,"side":"right","body":"nope"}`)
	require.Error(t, err)
	require.Empty(t, f.writer.opens)
}

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
