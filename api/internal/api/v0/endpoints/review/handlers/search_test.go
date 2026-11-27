package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type searchEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Hits      []gitdomain.SearchHit `json:"hits"`
		Truncated bool                  `json:"truncated"`
	} `json:"data"`
}

func getSearch(
	r http.Handler,
	query string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat1/review/search"+query, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}

func captureSearch(
	t *testing.T,
	query string,
) (string, gitdomain.SearchOpts) {
	t.Helper()
	var gotQuery string
	var gotOpts gitdomain.SearchOpts
	uc := stubUsecase{
		searchFn: func(_ context.Context, _, _, q string, opts gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			gotQuery, gotOpts = q, opts
			return nil, false, nil
		},
	}
	rec := getSearch(routerFor(uc), query)
	require.Equal(t, http.StatusOK, rec.Code)
	return gotQuery, gotOpts
}

func TestReviewHandlers_SearchDiff(t *testing.T) {
	rec := getSearch(newRouter(), "?q=todo")

	require.Equal(t, http.StatusOK, rec.Code)
	var env searchEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Len(t, env.Data.Hits, 1)
	assert.Equal(t, "a.go", env.Data.Hits[0].Path)
	assert.Equal(t, gitdomain.SearchSideNew, env.Data.Hits[0].Side)
	assert.Equal(t, 12, env.Data.Hits[0].LineNumber)
	assert.False(t, env.Data.Truncated)
}

func TestReviewHandlers_SearchDiff_Defaults(t *testing.T) {
	query, opts := captureSearch(t, "?q=todo")

	assert.Equal(t, "todo", query)
	assert.False(t, opts.Regex)
	assert.False(t, opts.CaseSensitive)
	assert.Equal(t, 200, opts.Limit)
}

func TestReviewHandlers_SearchDiff_ParsesFlags(t *testing.T) {
	_, opts := captureSearch(t, "?q=to.o&regex=true&case=true&limit=25")

	assert.True(t, opts.Regex)
	assert.True(t, opts.CaseSensitive)
	assert.Equal(t, 25, opts.Limit)
}

func TestReviewHandlers_SearchDiff_LimitIsAlwaysBounded(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"?q=e&limit=1", 1},
		{"?q=e&limit=1000", 1000},
		{"?q=e&limit=5000", 1000},
		{"?q=e&limit=0", 200},
		{"?q=e&limit=-7", 200},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, opts := captureSearch(t, tc.query)
			assert.Equal(t, tc.want, opts.Limit)
		})
	}
}

func TestReviewHandlers_SearchDiff_TruncatedIsReported(t *testing.T) {
	uc := stubUsecase{
		searchFn: func(_ context.Context, _, _, _ string, _ gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			return []gitdomain.SearchHit{{Path: "a.go"}}, true, nil
		},
	}
	rec := getSearch(routerFor(uc), "?q=e&limit=1")

	require.Equal(t, http.StatusOK, rec.Code)
	var env searchEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Data.Truncated)
}

func TestReviewHandlers_SearchDiff_EmptyQueryIsAnEmptyResult(t *testing.T) {
	uc := stubUsecase{
		searchFn: func(_ context.Context, _, _, _ string, _ gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			return nil, false, nil
		},
	}
	rec := getSearch(routerFor(uc), "?q=")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"hits":[]`)
}

func TestReviewHandlers_SearchDiff_MalformedParamsAreBadRequests(t *testing.T) {
	for _, query := range []string{"?q=e&regex=maybe", "?q=e&case=maybe", "?q=e&limit=lots"} {
		rec := getSearch(newRouter(), query)
		assert.Equalf(t, http.StatusBadRequest, rec.Code, "query %q", query)
	}
}

func TestReviewHandlers_SearchDiff_InvalidRegexIsBadRequest(t *testing.T) {
	uc := stubUsecase{
		searchErr: fmt.Errorf("branch review: search: %w: bad pattern", problem.ErrInvalidArgument),
	}
	rec := getSearch(routerFor(uc), "?q=a(b&regex=true")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReviewHandlers_SearchDiff_NotFound(t *testing.T) {
	rec := getSearch(routerFor(stubUsecase{searchErr: problem.ErrNotFound}), "?q=e")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
