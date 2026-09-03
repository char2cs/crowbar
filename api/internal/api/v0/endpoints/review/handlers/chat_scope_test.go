package handlers_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// This file pins the handler half of review's move onto /v0/chats/:chatId
// (spec §4.2's shared bucket, §8 step 4c): WHICH worktree a handler acts on,
// for each of the two prefixes the surface is currently mounted at.
//
// review has no WorkspaceReader seam (unlike search/identity): every method
// read ctx.Param("wsId") directly and handed it straight to the usecase. On a
// chat-scoped route that param does not exist, so an unguarded handler would
// call the usecase with the EMPTY string — not a 404, not a panic, just every
// review read aimed at nothing. Asserting a status code would not have caught
// it; only asserting the id the usecase was actually handed does.

// recordingUsecase is a ReviewUsecase that records which workspace id each
// call was made with.
type recordingUsecase struct {
	seen []string
}

func (r *recordingUsecase) Get(
	_ context.Context,
	wsID string,
) (domain.BranchReview, error) {
	r.seen = append(r.seen, wsID)
	return domain.BranchReview{}, nil
}

func (r *recordingUsecase) GetFiles(
	_ context.Context,
	wsID string,
	_ string,
) ([]gitdomain.ReviewFileSummary, error) {
	r.seen = append(r.seen, wsID)
	return nil, nil
}

func (r *recordingUsecase) SetMergeStrategy(
	_ context.Context,
	wsID string,
	_ gitdomain.MergeStrategy,
) error {
	r.seen = append(r.seen, wsID)
	return nil
}

func (r *recordingUsecase) GetOutline(
	_ context.Context,
	wsID string,
	_ string,
) ([]gitdomain.FileOutline, error) {
	r.seen = append(r.seen, wsID)
	return nil, nil
}

func (r *recordingUsecase) GetPatch(
	_ context.Context,
	wsID string,
	_ string,
	_ string,
	_ int,
	_ io.Writer,
) (int, bool, error) {
	r.seen = append(r.seen, wsID)
	return 0, false, nil
}

func (r *recordingUsecase) SearchDiff(
	_ context.Context,
	wsID string,
	_ string,
	_ string,
	_ gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	r.seen = append(r.seen, wsID)
	return nil, false, nil
}

// resolvedWorkspace is the workspace the chat group's resolveChatWorktree
// middleware stashes for chat "chat-1". Its id deliberately differs from every
// chat id used below: a handler that reached for the chat id instead of
// resolving it would pass this test's siblings by luck otherwise.
func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", ProjectID: "p1", RepoID: "r1"}
}

// reviewRouterForScopes wires review's two live mounts the way router.go
// does, including the chat group's middleware — the piece that makes a
// chat-scoped request resolvable at all.
func reviewRouterForScopes(
	t *testing.T,
	uc handlers.ReviewUsecase,
) *gin.Engine {
	t.Helper()
	h := handlers.New(uc)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/review", h.Get)
	chatScoped.GET("/review/files", h.GetFiles)

	wsScoped := r.Group("/v0")
	wsScoped.GET("/workspaces/:wsId/review", h.Get)
	wsScoped.GET("/workspaces/:wsId/review/files", h.GetFiles)

	return r
}

func doReviewRequest(
	t *testing.T,
	r *gin.Engine,
	method, path string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}

// TestChatScoped_ActsOnTheResolvedWorktreeNotTheChat is the core assertion: a
// read reached through a chat operates on the WORKSPACE that chat resolves
// to. Neither the chat id nor the empty string is an acceptable answer, and
// both are what the obvious wrong implementations produce.
func TestChatScoped_ActsOnTheResolvedWorktreeNotTheChat(t *testing.T) {
	uc := &recordingUsecase{}
	r := reviewRouterForScopes(t, uc)

	rec := doReviewRequest(t, r, http.MethodGet, "/v0/chats/chat-1/review")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, uc.seen)
	got := uc.seen[len(uc.seen)-1]
	assert.Equal(t, "ws-resolved", got)
	assert.NotEqual(t, "chat-1", got, "the chat id is not a workspace id")
	assert.NotEmpty(t, got, "an unresolved :wsId would silently aim review at nothing")
}

// TestWorkspaceScopedRoutesStillActOnTheirPathParam is the regression bar for
// the mount this step deliberately leaves standing: the old route keeps
// naming its workspace directly, and reqscope — never set on that group —
// must not have displaced it.
func TestWorkspaceScopedRoutesStillActOnTheirPathParam(t *testing.T) {
	uc := &recordingUsecase{}
	r := reviewRouterForScopes(t, uc)

	rec := doReviewRequest(t, r, http.MethodGet, "/v0/workspaces/ws-direct/review/files")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, uc.seen)
	assert.Equal(t, "ws-direct", uc.seen[len(uc.seen)-1])
}

// TestBothMountsReachTheSameHandlerSet proves the two prefixes are one
// surface rather than two implementations kept in step by hand: the same
// handler value answers both, so a behaviour change lands on both at once.
func TestBothMountsReachTheSameHandlerSet(t *testing.T) {
	uc := &recordingUsecase{}
	r := reviewRouterForScopes(t, uc)

	require.Equal(t, http.StatusOK,
		doReviewRequest(t, r, http.MethodGet, "/v0/chats/chat-1/review").Code)
	require.Equal(t, http.StatusOK,
		doReviewRequest(t, r, http.MethodGet, "/v0/workspaces/ws-resolved/review").Code)

	require.Len(t, uc.seen, 2)
	assert.Equal(t, uc.seen[0], uc.seen[1],
		"one worktree named two ways must reach the review usecase identically")
}
