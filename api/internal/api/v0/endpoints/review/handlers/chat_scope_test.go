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

func (r *recordingUsecase) GetFileSides(
	_ context.Context,
	wsID string,
	_ string,
	_ string,
	_ string,
) (gitdomain.ReviewFileSides, error) {
	r.seen = append(r.seen, wsID)
	return gitdomain.ReviewFileSides{}, nil
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

func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", ProjectID: "p1", RepoID: "r1"}
}

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

func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	uc := &recordingUsecase{}
	r := reviewRouterForScopes(t, uc)

	rec := doReviewRequest(t, r, http.MethodGet, "/v0/workspaces/ws-direct/review/files")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
