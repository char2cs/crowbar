package review_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubUsecase struct{}

func (stubUsecase) Get(
	_ context.Context,
	_ string,
) (domain.BranchReview, error) {
	return domain.BranchReview{}, nil
}

func (stubUsecase) GetFiles(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.ReviewFileSummary, error) {
	return nil, nil
}

func (stubUsecase) SetMergeStrategy(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) error {
	return nil
}

func (stubUsecase) GetOutline(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.FileOutline, error) {
	return nil, nil
}

func (stubUsecase) GetPatch(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ int,
	_ io.Writer,
) (int, bool, error) {
	return 0, false, nil
}

func (stubUsecase) SearchDiff(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	return nil, false, nil
}

// reviewSurface is the method+relative-path set review.Register mounts,
// written once and asserted against BOTH live prefixes (mirroring git's
// routes_test.go, spec §8 step 4b's reference implementation for this step).
func reviewSurface() []struct {
	method string
	path   string
} {
	return []struct {
		method string
		path   string
	}{
		{http.MethodGet, ""},
		{http.MethodGet, "/files"},
		{http.MethodGet, "/outline"},
		{http.MethodGet, "/patch?path=a.go"},
		{http.MethodGet, "/search?q=todo"},
		{http.MethodPatch, ""},
	}
}

// registerChatScoped wires review.Register the way router.go does: on the
// flat chat-scoped group alone (spec §8 step 6 retired the old
// workspace-scoped mount).
func registerChatScoped(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	review.Register(v0.Group("/chats/:chatId"), stubUsecase{})
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every
// review route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(
	t *testing.T,
) {
	r := registerChatScoped(t)

	for _, tc := range reviewSurface() {
		path := "/v0/chats/chat1/review" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterDropsWorkspaceScopedRoutes proves spec §8 step 6's deletion is
// real for review: the old /v0/workspaces/:wsId/review... mount, kept alive
// alongside the chat-scoped one through the rest of this refactor, answers
// nothing any more.
func TestRegisterDropsWorkspaceScopedRoutes(
	t *testing.T,
) {
	r := registerChatScoped(t)

	for _, tc := range reviewSurface() {
		path := "/v0/workspaces/ws1/review" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}
