package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func getContents(
	h http.Handler,
	query string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v0/chats/chat1/review/contents"+query, http.NoBody))
	return rec
}

func TestReviewHandlers_GetFileSides_ServesBothSidesForTheScopedFile(t *testing.T) {
	var gotCommit, gotPath, gotOld string
	old, next := "a\n", "b\n"
	uc := stubUsecase{
		sides: func(_ context.Context, _, commit, path, oldPath string) (gitdomain.ReviewFileSides, error) {
			gotCommit, gotPath, gotOld = commit, path, oldPath
			return gitdomain.ReviewFileSides{Old: &old, New: &next}, nil
		},
	}

	rec := getContents(routerFor(uc), "?path=src/a.go&oldPath=src/old.go&sha=abc123")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "abc123", gotCommit)
	assert.Equal(t, "src/a.go", gotPath)
	assert.Equal(t, "src/old.go", gotOld)
	var env struct {
		Data gitdomain.ReviewFileSides `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "a\n", *env.Data.Old)
	assert.Equal(t, "b\n", *env.Data.New)
}

func TestReviewHandlers_GetFileSides_RequiresAPath(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, getContents(newRouter(), "").Code)
}

func TestReviewHandlers_GetFileSides_OversizedFileIs422(t *testing.T) {
	r := routerFor(errUsecase{err: apperr.ErrUnprocessable})

	assert.Equal(t, http.StatusUnprocessableEntity, getContents(r, "?path=big.go").Code)
}
