package review_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
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

func (stubUsecase) SetMergeStrategy(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) error {
	return nil
}

func (stubUsecase) OpenThread(
	_ context.Context,
	_ branchreview.OpenThreadInput,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubUsecase) Reply(
	_ context.Context,
	_ string,
	_ string,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (stubUsecase) SetThreadResolved(
	_ context.Context,
	_ string,
	_ bool,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	review.Register(r.Group("/v0"), stubUsecase{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/workspaces/ws1/review"},
		{http.MethodPatch, "/v0/workspaces/ws1/review"},
		{http.MethodPost, "/v0/workspaces/ws1/review/threads"},
		{http.MethodPost, "/v0/workspaces/ws1/review/threads/t1/reply"},
		{http.MethodPatch, "/v0/workspaces/ws1/review/threads/t1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
