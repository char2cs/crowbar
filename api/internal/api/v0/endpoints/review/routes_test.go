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
) ([]gitdomain.FileOutline, error) {
	return nil, nil
}

func (stubUsecase) GetPatch(
	_ context.Context,
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
	_ gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	return nil, false, nil
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
		{http.MethodGet, "/v0/workspaces/ws1/review/files"},
		{http.MethodGet, "/v0/workspaces/ws1/review/outline"},
		{http.MethodGet, "/v0/workspaces/ws1/review/patch?path=a.go"},
		{http.MethodGet, "/v0/workspaces/ws1/review/search?q=todo"},
		{http.MethodPatch, "/v0/workspaces/ws1/review"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
