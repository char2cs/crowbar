package review_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type stubReview struct{}

func (stubReview) Get(
	_ context.Context,
	_ string,
) (domain.BranchReview, error) {
	return domain.BranchReview{}, nil
}

func (stubReview) SetMergeStrategy(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) error {
	return nil
}

func (stubReview) OpenThread(
	_ context.Context,
	_ branchreview.OpenThreadInput,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubReview) Reply(
	_ context.Context,
	_ string,
	_ string,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubReview) SetThreadResolved(
	_ context.Context,
	_ string,
	_ bool,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	review.Register(
		r.Group("/v0"),
		stubReview{},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/review", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	want := map[string]string{
		"GET /v0/workspaces/:wsId/review":                    "",
		"PATCH /v0/workspaces/:wsId/review":                  "",
		"POST /v0/workspaces/:wsId/review/threads":           "",
		"POST /v0/workspaces/:wsId/review/threads/:id/reply": "",
		"PATCH /v0/workspaces/:wsId/review/threads/:id":      "",
	}
	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}
	for key := range want {
		assert.True(t, mounted[key], "route %s not mounted", key)
	}
}
