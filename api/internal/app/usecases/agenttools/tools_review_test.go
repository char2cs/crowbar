package agenttools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

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
// method was last asked for.
type stubReviewReader struct {
	base     string
	files    []gitdomain.ReviewFileSummary
	lastWsID string
}

func (s *stubReviewReader) GetBase(_ context.Context, wsID string) (string, error) {
	s.lastWsID = wsID
	return s.base, nil
}

func (s *stubReviewReader) GetFiles(_ context.Context, wsID string, _ string) ([]gitdomain.ReviewFileSummary, error) {
	s.lastWsID = wsID
	return s.files, nil
}

func (s *stubReviewReader) GetOutline(_ context.Context, wsID string, _ string) ([]gitdomain.FileOutline, error) {
	s.lastWsID = wsID
	return nil, nil
}

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
	deps := agenttools.Deps{Resolver: res, Threads: threads, Review: review}
	return agenttools.NewToolSet(deps, "RUN", tok), tok
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
