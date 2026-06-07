package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestGet_ReturnsCompositeEnvelope(t *testing.T) {
	stub := &stubReview{
		review: domain.BranchReview{
			Description:   "desc",
			MergeStrategy: gitdomain.MergeStrategySquash,
			Diff:          gitdomain.MultiFileDiff{},
			Threads: []domain.ReviewThread{
				{
					ID:        "t1",
					WsID:      "ws1",
					FilePath:  "a.go",
					Side:      domain.ReviewSideRight,
					Status:    domain.ReviewThreadStatusOpen,
					CreatedAt: time.Unix(0, 0).UTC(),
				},
			},
		},
	}
	r := newRouter(stub)

	w := do(t, r, http.MethodGet, "/v0/workspaces/ws1/review", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ws1", stub.lastWsID)

	env := decode(t, w)
	require.True(t, env.Success)
	require.Empty(t, env.Error)

	var got dto.BranchReviewDTO
	require.NoError(t, jsonUnmarshal(env.Data, &got))
	assert.Equal(t, "desc", got.Description)
	assert.Equal(t, string(gitdomain.MergeStrategySquash), got.MergeStrategy)
	require.Len(t, got.Threads, 1)
	assert.NotNil(t, got.Threads[0].Messages)
	assert.NotNil(t, got.Conversations)
	assert.NotNil(t, got.Diff.Files)
}

func TestGet_NotFound_404(t *testing.T) {
	stub := &stubReview{getErr: apperr.ErrNotFound}
	r := newRouter(stub)

	w := do(t, r, http.MethodGet, "/v0/workspaces/nope/review", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	env := decode(t, w)
	assert.False(t, env.Success)
	assert.NotEmpty(t, env.Error)
}

func TestGet_InternalFailure_500(t *testing.T) {
	stub := &stubReview{getErr: assertAnError()}
	r := newRouter(stub)

	w := do(t, r, http.MethodGet, "/v0/workspaces/ws1/review", nil)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	env := decode(t, w)
	assert.False(t, env.Success)
}

func TestSetMergeStrategy_OK(t *testing.T) {
	stub := &stubReview{}
	r := newRouter(stub)

	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review",
		map[string]any{"mergeStrategy": string(gitdomain.MergeStrategySquash)})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, gitdomain.MergeStrategySquash, stub.lastStrategy)

	env := decode(t, w)
	require.True(t, env.Success)
	var data map[string]string
	require.NoError(t, jsonUnmarshal(env.Data, &data))
	assert.Equal(t, string(gitdomain.MergeStrategySquash), data["mergeStrategy"])
}

func TestSetMergeStrategy_MissingField_400(t *testing.T) {
	stub := &stubReview{}
	r := newRouter(stub)

	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review", map[string]any{})
	require.Equal(t, http.StatusBadRequest, w.Code)
	env := decode(t, w)
	assert.False(t, env.Success)
}

func TestSetMergeStrategy_BadJSON_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review", []byte("{"))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSetMergeStrategy_NotFound_404(t *testing.T) {
	stub := &stubReview{stratErr: apperr.ErrNotFound}
	r := newRouter(stub)

	w := do(t, r, http.MethodPatch, "/v0/workspaces/nope/review",
		map[string]any{"mergeStrategy": string(gitdomain.MergeStrategySquash)})
	require.Equal(t, http.StatusNotFound, w.Code)
}
