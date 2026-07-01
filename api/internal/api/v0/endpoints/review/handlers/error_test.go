package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type errUsecase struct{ err error }

func (e errUsecase) Get(_ context.Context, _ string) (domain.BranchReview, error) {
	return domain.BranchReview{}, e.err
}

func (e errUsecase) SetMergeStrategy(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) error {
	return e.err
}

func newErrRouter(uc handlers.ReviewUsecase) *gin.Engine {
	r := gin.New()
	h := handlers.New(uc)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/review", h.Get)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
	return r
}

func TestReviewHandlers_NotFound(
	t *testing.T,
) {
	r := newErrRouter(errUsecase{err: apperr.ErrNotFound})

	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, "/v0/workspaces/ws1/review", nil).Code)
}

func TestReviewHandlers_InternalError(
	t *testing.T,
) {
	r := newErrRouter(errUsecase{err: errors.New("boom")})

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, "/v0/workspaces/ws1/review", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPatch, "/v0/workspaces/ws1/review",
		map[string]any{"mergeStrategy": "merge"}).Code)
}
