package agenttools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// stubThreadReader is the ThreadReader test double: it hands back a fixed
// list and records the wsID it was last asked for, which is how the
// caller-scoping tests prove a tool cannot be steered at another workspace.
type stubThreadReader struct {
	list     []domain.ReviewThread
	lastWsID string
}

func (s *stubThreadReader) ListByWorkspace(_ context.Context, wsID string) ([]domain.ReviewThread, error) {
	s.lastWsID = wsID
	return s.list, nil
}

func (s *stubThreadReader) Get(_ context.Context, id string) (domain.ReviewThread, error) {
	for _, t := range s.list {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.ReviewThread{}, apperrNotFound()
}

// stubReviewReader is the ReviewReader test double, recording the wsID each
// method was last asked for. outline is what GetOutline hands back, which is the
// diff geometry post_review_comment validates every anchor against.
type stubReviewReader struct {
	base     string
	files    []gitdomain.ReviewFileSummary
	outline  []gitdomain.FileOutline
	lastWsID string
}

func (s *stubReviewReader) GetBase(_ context.Context, wsID string) (string, error) {
	s.lastWsID = wsID
	return s.base, nil
}

func (s *stubReviewReader) GetFiles(_ context.Context, wsID, _ string) ([]gitdomain.ReviewFileSummary, error) {
	s.lastWsID = wsID
	return s.files, nil
}

func (s *stubReviewReader) GetOutline(_ context.Context, wsID, _ string) ([]gitdomain.FileOutline, error) {
	s.lastWsID = wsID
	return s.outline, nil
}

// stubThreadWriter is the ThreadWriter test double. It records EVERY Open input,
// which is what lets the rejection tests assert the store was never written to
// rather than merely that an error came back — an implementation that validated
// after writing would return the same error and still leave the floating comment.
type stubThreadWriter struct {
	opens  []reviewthread.OpenInput
	nextID int
	err    error
}

func (s *stubThreadWriter) Open(
	_ context.Context,
	in reviewthread.OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	if s.err != nil {
		return domain.ReviewThread{}, s.err
	}
	s.opens = append(s.opens, in)
	s.nextID++
	return domain.ReviewThread{
		ID:        fmt.Sprintf("thread-%d", s.nextID),
		WsID:      in.WsID,
		FilePath:  in.FilePath,
		StartLine: in.StartLine,
		EndLine:   in.EndLine,
		Side:      in.Side,
		Status:    domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID: in.MessageID, Author: in.Author, IsAgent: in.IsAgent, Body: in.Body, CreatedAt: now,
		}},
		CreatedAt: now,
	}, nil
}

func (s *stubThreadWriter) Reply(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ bool,
	_ string,
	_ time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (s *stubThreadWriter) Resolve(_ context.Context, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

// spyThreadBroadcast is the ThreadBroadcast test double. Fan-out is the only thing
// that puts an agent's finding in front of a user who is already looking at the
// review pane, so every test that stores a comment also checks the frame, and every
// test that rejects one checks that no frame was emitted.
type spyThreadBroadcast struct {
	frames []broadcastFrame
}

type broadcastFrame struct {
	thread    domain.ReviewThread
	projectID string
	repoID    string
}

func (s *spyThreadBroadcast) fn() agenttools.ThreadBroadcast {
	return func(thread domain.ReviewThread, projectID, repoID string) {
		s.frames = append(s.frames, broadcastFrame{thread: thread, projectID: projectID, repoID: repoID})
	}
}

// callerProviderID is the provider the fixture's runner is on. post_review_comment
// attributes findings to it, so the tests can prove the author came from the
// runner rather than from a constant.
const callerProviderID = "claude"

// reviewToolsetOn builds a ToolSet with the given review-surface deps on a
// caller resolved to ws-a, mirroring toolsetOn's fixture but letting each
// review test control the review deps it cares about independently.
func reviewToolsetOn(
	t *testing.T,
	threads agenttools.ThreadReader,
	review agenttools.ReviewReader,
) (*agenttools.ToolSet, string) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
	deps := agenttools.Deps{
		Resolver:        res,
		Threads:         threads,
		Review:          review,
		ThreadWrites:    &stubThreadWriter{},
		Idempotency:     agenttools.NewIdempotency(),
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}
	return agenttools.NewToolSet(deps, "RUN", tok), tok
}

// postFixture is the post_review_comment write surface: one caller plus every
// double it writes through, so a test can assert on the store, the fan-out and the
// outline reader together.
type postFixture struct {
	ts        *agenttools.ToolSet
	writer    *stubThreadWriter
	review    *stubReviewReader
	broadcast *spyThreadBroadcast
	idem      *agenttools.Idempotency
}

// postOn builds a fresh fixture whose caller resolves to callerWs and whose review
// contains outline.
func postOn(
	t *testing.T,
	callerWs string,
	outline []gitdomain.FileOutline,
) *postFixture {
	t.Helper()
	return newPostFixture(
		t, callerWs, outline,
		&stubThreadWriter{}, agenttools.NewIdempotency(), &spyThreadBroadcast{},
	)
}

// retryOn builds a SECOND fixture sharing this one's store, dedup map and
// broadcaster. That is the production shape of a retry: a ToolSet is built per MCP
// request, so the original call and its retry are served by different ToolSets over
// the same long-lived dependencies.
func (f *postFixture) retryOn(
	t *testing.T,
	callerWs string,
	outline []gitdomain.FileOutline,
) *postFixture {
	t.Helper()
	return newPostFixture(t, callerWs, outline, f.writer, f.idem, f.broadcast)
}

func newPostFixture(
	t *testing.T,
	callerWs string,
	outline []gitdomain.FileOutline,
	writer *stubThreadWriter,
	idem *agenttools.Idempotency,
	broadcast *spyThreadBroadcast,
) *postFixture {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{
			ID:            "RUN",
			CurrentChatID: "CHAT",
			WorkspaceID:   callerWs,
			ProviderID:    callerProviderID,
		}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	review := &stubReviewReader{outline: outline}
	deps := agenttools.Deps{
		Resolver:        res,
		Threads:         &stubThreadReader{},
		Review:          review,
		ThreadWrites:    writer,
		Idempotency:     idem,
		ThreadBroadcast: broadcast.fn(),
	}
	return &postFixture{
		ts:        agenttools.NewToolSet(deps, "RUN", m.Mint("RUN")),
		writer:    writer,
		review:    review,
		broadcast: broadcast,
		idem:      idem,
	}
}

func (f *postFixture) post(args string) (string, error) {
	return f.ts.Call(context.Background(), "post_review_comment", json.RawMessage(args))
}

// outlineWithHunk is the smallest review a post can anchor into: one file, one
// hunk.
func outlineWithHunk(path string, hunk gitdomain.HunkShape) []gitdomain.FileOutline {
	return []gitdomain.FileOutline{{Path: path, Hunks: []gitdomain.HunkShape{hunk}}}
}

// authHunk is the fixture hunk every anchor test measures against: on the right it
// covers lines 40..49 inclusive.
func authHunk() []gitdomain.FileOutline {
	return outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
}

func TestListReviewThreads_DefaultsToUnresolvedOnly(t *testing.T) {
	open := domain.ReviewThread{
		ID: "open-1", WsID: "ws-a", FilePath: "a.go",
		StartLine: 1, EndLine: 1, Side: domain.ReviewSideRight,
		Status: domain.ReviewThreadStatusOpen,
	}
	resolved := domain.ReviewThread{
		ID: "resolved-1", WsID: "ws-a", FilePath: "b.go",
		StartLine: 2, EndLine: 2, Side: domain.ReviewSideRight,
		Status: domain.ReviewThreadStatusResolved,
	}
	stub := &stubThreadReader{list: []domain.ReviewThread{open, resolved}}
	ts, _ := reviewToolsetOn(t, stub, &stubReviewReader{})

	out, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, out, "open-1")
	require.NotContains(t, out, "resolved-1")

	out, err = ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{"includeResolved":true}`))
	require.NoError(t, err)
	require.Contains(t, out, "open-1")
	require.Contains(t, out, "resolved-1")
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
}

// TestReviewTools_OnlyReadTheCallersOwnWorkspace is the security property: both
// tools take no workspace-like argument at all, so the ONLY wsID either can
// ever query is the one the Resolver computed for the caller.
func TestReviewTools_OnlyReadTheCallersOwnWorkspace(t *testing.T) {
	threadStub := &stubThreadReader{}
	reviewStub := &stubReviewReader{}
	ts, _ := reviewToolsetOn(t, threadStub, reviewStub)

	_, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ws-a", threadStub.lastWsID)

	_, err = ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ws-a", reviewStub.lastWsID)

	forbidden := []string{"wsId", "wsID", "workspaceId", "workspace_id"}
	for _, tool := range ts.Tools() {
		if tool.Name != "list_review_threads" && tool.Name != "get_review_scope" {
			continue
		}
		for _, f := range forbidden {
			require.NotContains(t, string(tool.InputSchema), f,
				"tool %s exposes %s; scope must never be an argument", tool.Name, f)
		}
	}
}

func TestPostReviewComment_AnchorsAndMarksItselfAsAgent(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	out, err := f.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":"This leaks the token."}`)
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
				tc.line, tc.line))
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
				tc.start, tc.end))
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
			`{"filePath":"src/auth.go","startLine":%d,"endLine":%d,"side":"left","body":"x"}`, line, line))
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
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"k"}`)
	require.NoError(t, err)

	out, err := first.retryOn(t, "ws-a", authHunk()).post(
		`{"filePath":"src/auth.go","startLine":47,"endLine":48,"side":"right","body":"leak","idempotencyKey":"k"}`)
	require.NoError(t, err)

	require.Len(t, first.writer.opens, 1)
	require.Contains(t, out, "42-42", "the reply must describe the thread that exists")
	require.NotContains(t, out, "47-48", "reporting the unstored arguments would be a lie")
}

func TestPostReviewComment_DifferentKeysOpenDifferentThreads(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"one","idempotencyKey":"finding-a"}`)
	require.NoError(t, err)
	_, err = f.post(
		`{"filePath":"src/auth.go","startLine":43,"endLine":43,"side":"right","body":"two","idempotencyKey":"finding-b"}`)
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
	full := func() agenttools.Deps {
		return agenttools.Deps{
			Review:          &stubReviewReader{outline: authHunk()},
			ThreadWrites:    &stubThreadWriter{},
			Idempotency:     agenttools.NewIdempotency(),
			ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
		}
	}
	cases := map[string]func(*agenttools.Deps){
		"no review reader": func(d *agenttools.Deps) { d.Review = nil },
		"no thread writer": func(d *agenttools.Deps) { d.ThreadWrites = nil },
		"no dedup map":     func(d *agenttools.Deps) { d.Idempotency = nil },
		"no broadcaster":   func(d *agenttools.Deps) { d.ThreadBroadcast = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			deps := full()
			drop(&deps)
			m, err := agenttools.NewTokenMinter()
			require.NoError(t, err)
			deps.Resolver = agenttools.NewResolver(m,
				stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
				stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
				stubWorkspaces{all: tree()})
			ts := agenttools.NewToolSet(deps, "RUN", m.Mint("RUN"))
			for _, tool := range ts.Tools() {
				require.NotEqual(t, "post_review_comment", tool.Name)
			}
			_, err = ts.Call(context.Background(), "post_review_comment", json.RawMessage(`{}`))
			require.Error(t, err)
		})
	}
}
