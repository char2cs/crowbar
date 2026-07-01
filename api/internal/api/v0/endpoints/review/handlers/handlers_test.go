package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
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

func newRouter() *gin.Engine {
	r := gin.New()
	h := handlers.New(stubUsecase{})
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/review", h.Get)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
	return r
}

func do(
	r *gin.Engine,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestReviewHandlers_HappyPath(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/review", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPatch, "/v0/workspaces/ws1/review",
		map[string]any{"mergeStrategy": "merge"})
	assert.Equal(t, http.StatusOK, rec.Code)
}
