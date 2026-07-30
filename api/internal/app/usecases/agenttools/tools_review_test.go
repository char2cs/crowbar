package agenttools_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	ts, _, tok := reviewToolsetWithWriter(t, "ws-a", threads, review, &stubThreadWriter{}, agenttools.NewIdempotency())
	return ts, tok
}

// reviewToolsetWithWriter is the write-surface fixture: it resolves the caller to
// callerWs and returns the ToolSet together with the writer it will post through.
// The Idempotency is a parameter so two ToolSets can be built over ONE dedup map,
// which is what a retry looks like in production (a ToolSet is per request).
func reviewToolsetWithWriter(
	t *testing.T,
	callerWs string,
	threads agenttools.ThreadReader,
	review agenttools.ReviewReader,
	writer *stubThreadWriter,
	idem *agenttools.Idempotency,
) (*agenttools.ToolSet, *stubThreadWriter, string) {
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
	tok := m.Mint("RUN")
	deps := agenttools.Deps{
		Resolver:     res,
		Threads:      threads,
		Review:       review,
		ThreadWrites: writer,
		Idempotency:  idem,
	}
	return agenttools.NewToolSet(deps, "RUN", tok), writer, tok
}

// outlineWithHunk is the smallest review a post can anchor into: one file, one
// hunk.
func outlineWithHunk(path string, hunk gitdomain.HunkShape) []gitdomain.FileOutline {
	return []gitdomain.FileOutline{{Path: path, Hunks: []gitdomain.HunkShape{hunk}}}
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
	review := &stubReviewReader{
		outline: outlineWithHunk("src/auth.go", gitdomain.HunkShape{
			OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
		}),
	}
	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, review, &stubThreadWriter{}, agenttools.NewIdempotency())

	out, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":"This leaks the token."}`))
	require.NoError(t, err)
	require.Contains(t, out, "thread-1")

	require.Len(t, writer.opens, 1)
	got := writer.opens[0]
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
	require.NotEmpty(t, got.Author, "an unattributed comment renders as a blank name in the review UI")
	require.Equal(t, callerProviderID, got.Author, "the author must come from the caller's runner provider")
	require.NotEmpty(t, got.ID)
	require.NotEmpty(t, got.MessageID)
	require.NotEqual(t, got.ID, got.MessageID)
}

// The whole correctness risk: an anchor outside any hunk floats off the diff, so
// the user sees a finding with no code beside it. The assertion that matters is
// that NOTHING was written, not that an error came back.
func TestPostReviewComment_RejectsAnAnchorOutsideAnyHunk(t *testing.T) {
	review := &stubReviewReader{
		outline: outlineWithHunk("src/auth.go", gitdomain.HunkShape{
			OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
		}),
	}
	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, review, &stubThreadWriter{}, agenttools.NewIdempotency())

	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":200,"endLine":200,"side":"right","body":"nope"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "get_review_scope")
	require.Empty(t, writer.opens, "a rejected anchor must never reach the thread store")
}

// A range that starts inside the hunk but runs past its end is still floating for
// most of its length, so it is rejected whole rather than silently clamped.
func TestPostReviewComment_RejectsARangeThatOverrunsTheHunk(t *testing.T) {
	review := &stubReviewReader{
		outline: outlineWithHunk("src/auth.go", gitdomain.HunkShape{
			OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
		}),
	}
	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, review, &stubThreadWriter{}, agenttools.NewIdempotency())

	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":48,"endLine":60,"side":"right","body":"nope"}`))
	require.Error(t, err)
	require.Empty(t, writer.opens)
}

func TestPostReviewComment_RejectsAnUnknownFile(t *testing.T) {
	review := &stubReviewReader{
		outline: outlineWithHunk("src/auth.go", gitdomain.HunkShape{
			OldStart: 1, OldLines: 100, NewStart: 1, NewLines: 100,
		}),
	}
	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, review, &stubThreadWriter{}, agenttools.NewIdempotency())

	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/untouched.go","startLine":5,"endLine":5,"side":"right","body":"nope"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "src/untouched.go")
	require.Contains(t, err.Error(), "get_review_scope")
	require.Empty(t, writer.opens, "a file outside the review must never reach the thread store")
}

// The two numberings diverge by every insertion above them. The hunk here makes
// line 12 valid on the LEFT and invalid on the RIGHT, and line 105 the reverse, so
// an implementation that read the wrong pair of the hunk's four numbers fails both
// halves of this test.
func TestPostReviewComment_LeftSideAnchorsAgainstOldLineNumbers(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 10, OldLines: 5, NewStart: 100, NewLines: 20,
	})

	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline},
		&stubThreadWriter{}, agenttools.NewIdempotency())
	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":12,"endLine":12,"side":"left","body":"removed too much"}`))
	require.NoError(t, err, "line 12 is inside the hunk's OLD range")
	require.Len(t, writer.opens, 1)
	require.Equal(t, domain.ReviewSideLeft, writer.opens[0].Side)

	_, err = ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":12,"endLine":12,"side":"right","body":"wrong side"}`))
	require.Error(t, err, "line 12 is outside the hunk's NEW range")
	require.Len(t, writer.opens, 1, "the rejected post must not have been written")

	_, err = ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":105,"endLine":105,"side":"left","body":"wrong side"}`))
	require.Error(t, err, "line 105 is outside the hunk's OLD range")
	require.Len(t, writer.opens, 1)

	_, err = ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":105,"endLine":105,"side":"right","body":"added badly"}`))
	require.NoError(t, err, "line 105 is inside the hunk's NEW range")
	require.Len(t, writer.opens, 2)
	require.Equal(t, domain.ReviewSideRight, writer.opens[1].Side)
}

// A rename is addressed by either of its names, so a model that read the file
// under its old path can still anchor to it.
func TestPostReviewComment_AcceptsARenamedFileUnderEitherPath(t *testing.T) {
	review := &stubReviewReader{outline: []gitdomain.FileOutline{{
		Path:    "src/new_name.go",
		OldPath: "src/old_name.go",
		Hunks:   []gitdomain.HunkShape{{OldStart: 1, OldLines: 20, NewStart: 1, NewLines: 20}},
	}}}
	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, review, &stubThreadWriter{}, agenttools.NewIdempotency())

	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/old_name.go","startLine":3,"endLine":3,"side":"right","body":"here"}`))
	require.NoError(t, err)
	require.Len(t, writer.opens, 1)
}

// A binary file has no hunks at all, so there is no line for a comment to sit on.
func TestPostReviewComment_RejectsABinaryFile(t *testing.T) {
	review := &stubReviewReader{outline: []gitdomain.FileOutline{
		{Path: "assets/logo.png", IsBinary: true},
	}}
	ts, writer, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, review, &stubThreadWriter{}, agenttools.NewIdempotency())

	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"assets/logo.png","startLine":1,"endLine":1,"side":"right","body":"nope"}`))
	require.Error(t, err)
	require.Empty(t, writer.opens)
}

// The retry a dropped MCP response produces arrives on a NEW ToolSet, which is why
// the dedup map is a dependency rather than ToolSet state — two ToolSets over one
// Idempotency is exactly the production shape.
func TestPostReviewComment_IdempotencyKeyCollapsesARetry(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
	writer := &stubThreadWriter{}
	idem := agenttools.NewIdempotency()
	args := json.RawMessage(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"leak-in-auth"}`)

	first, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline}, writer, idem)
	out1, err := first.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)

	second, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline}, writer, idem)
	out2, err := second.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)

	require.Len(t, writer.opens, 1, "a retry with the same key must open exactly one thread")
	require.Contains(t, out1, "thread-1")
	require.Contains(t, out2, "thread-1", "the retry must report the thread the first call opened")
}

func TestPostReviewComment_DifferentKeysOpenDifferentThreads(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
	writer := &stubThreadWriter{}
	ts, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline},
		writer, agenttools.NewIdempotency())

	_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"one","idempotencyKey":"finding-a"}`))
	require.NoError(t, err)
	_, err = ts.Call(context.Background(), "post_review_comment", json.RawMessage(
		`{"filePath":"src/auth.go","startLine":43,"endLine":43,"side":"right","body":"two","idempotencyKey":"finding-b"}`))
	require.NoError(t, err)

	require.Len(t, writer.opens, 2)
}

// Keys are scoped by workspace: two agents reviewing two branches will invent the
// same obvious key, and the second finding must not be swallowed as a retry of the
// first. ws-a and ws-a1 are sibling review surfaces in the fixture tree.
func TestPostReviewComment_SameKeyInTwoWorkspacesDoesNotCollide(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
	writer := &stubThreadWriter{}
	idem := agenttools.NewIdempotency()
	args := json.RawMessage(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"leak-in-auth"}`)

	onA, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline}, writer, idem)
	_, err := onA.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)

	onA1, _, _ := reviewToolsetWithWriter(
		t, "ws-a1", &stubThreadReader{}, &stubReviewReader{outline: outline}, writer, idem)
	_, err = onA1.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)

	require.Len(t, writer.opens, 2, "the same key in a different workspace is a different finding")
	require.Equal(t, "ws-a", writer.opens[0].WsID)
	require.Equal(t, "ws-a1", writer.opens[1].WsID)
}

// No key means no dedup: a model that omits it gets a thread per call.
func TestPostReviewComment_WithoutAKeyEveryCallOpensAThread(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
	writer := &stubThreadWriter{}
	ts, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline},
		writer, agenttools.NewIdempotency())
	args := json.RawMessage(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak"}`)

	_, err := ts.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)
	_, err = ts.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)

	require.Len(t, writer.opens, 2)
}

// A failed write must not be remembered as done, or the retry the key exists for
// would return success having stored nothing.
func TestPostReviewComment_AFailedWriteIsNotRemembered(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
	writer := &stubThreadWriter{err: errNotFoundForTest}
	idem := agenttools.NewIdempotency()
	args := json.RawMessage(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right","body":"leak","idempotencyKey":"k"}`)

	ts, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline}, writer, idem)
	_, err := ts.Call(context.Background(), "post_review_comment", args)
	require.Error(t, err)

	writer.err = nil
	retry, _, _ := reviewToolsetWithWriter(
		t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline}, writer, idem)
	out, err := retry.Call(context.Background(), "post_review_comment", args)
	require.NoError(t, err)
	require.Contains(t, out, "thread-1")
	require.Len(t, writer.opens, 1)
}

func TestPostReviewComment_RejectsBadArguments(t *testing.T) {
	outline := outlineWithHunk("src/auth.go", gitdomain.HunkShape{
		OldStart: 40, OldLines: 10, NewStart: 40, NewLines: 10,
	})
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
			ts, writer, _ := reviewToolsetWithWriter(
				t, "ws-a", &stubThreadReader{}, &stubReviewReader{outline: outline},
				&stubThreadWriter{}, agenttools.NewIdempotency())
			_, err := ts.Call(context.Background(), "post_review_comment", json.RawMessage(args))
			require.Error(t, err)
			require.Empty(t, writer.opens)
		})
	}
}

// post_review_comment is the first WRITE tool, so its fail-closed wiring is worth
// asserting directly: with no outline reader it cannot validate an anchor, and with
// no dedup map a retry would duplicate a finding. Either way it must not exist.
func TestPostReviewComment_NotAdvertisedWithoutItsDependencies(t *testing.T) {
	outline := outlineWithHunk("a.go", gitdomain.HunkShape{NewStart: 1, NewLines: 2})
	cases := map[string]agenttools.Deps{
		"no review reader": {Review: nil, ThreadWrites: &stubThreadWriter{}, Idempotency: agenttools.NewIdempotency()},
		"no thread writer": {Review: &stubReviewReader{outline: outline}, ThreadWrites: nil, Idempotency: agenttools.NewIdempotency()},
		"no dedup map":     {Review: &stubReviewReader{outline: outline}, ThreadWrites: &stubThreadWriter{}, Idempotency: nil},
	}
	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
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
