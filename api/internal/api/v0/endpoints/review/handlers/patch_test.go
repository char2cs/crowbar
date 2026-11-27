package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/middleware"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/perf"
)

func getPatch(
	r http.Handler,
	rec *httptest.ResponseRecorder,
	query string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat1/review/patch"+query, http.NoBody)
	r.ServeHTTP(rec, req)
	return rec
}

func TestReviewHandlers_GetPatch(t *testing.T) {
	rec := getPatch(newRouter(), httptest.NewRecorder(), "?path=a.go")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Equal(t, "diff --git a/a.go b/a.go\n", rec.Body.String())
}

func TestReviewHandlers_GetPatch_StreamsIncrementallyRatherThanBuffering(t *testing.T) {
	rec := httptest.NewRecorder()

	const lines = 8
	const line = "+streamed line\n"
	var seen []int
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			for range lines {
				if _, err := io.WriteString(w, line); err != nil {
					return 0, false, err
				}
				seen = append(seen, rec.Body.Len())
			}
			return lines, false, nil
		},
	}

	getPatch(routerFor(uc), rec, "?path=a.go&maxLines=0")

	require.Equal(t, http.StatusOK, rec.Code)
	want := make([]int, 0, lines)
	for i := 1; i <= lines; i++ {
		want = append(want, i*len(line))
	}
	assert.Equal(t, want, seen,
		"each write must reach the response before the next is produced")
}

func TestReviewHandlers_GetPatch_StillStreamsThroughTheTimingWriter(t *testing.T) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	rec := httptest.NewRecorder()
	const lines = 4
	const line = "+streamed line\n"
	var seen []int
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			for range lines {
				if _, err := io.WriteString(w, line); err != nil {
					return 0, false, err
				}
				seen = append(seen, rec.Body.Len())
			}
			return lines, false, nil
		},
	}

	r := routerFor(uc)
	r.Use(middleware.Timing())
	getPatch(r, rec, "?path=a.go&maxLines=0")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []int{len(line), 2 * len(line), 3 * len(line), 4 * len(line)}, seen)
}

func TestReviewHandlers_GetPatch_ReportsTruncationAsAReadableHeader(t *testing.T) {
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			n, err := io.WriteString(w, "@@ -1,2 +1,2 @@\n")
			return n, true, err
		},
	}
	rec := getPatch(routerFor(uc), httptest.NewRecorder(), "?path=a.go&maxLines=1")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "true", rec.Result().Header.Get("X-Crowbar-Diff-Truncated"))
	assert.Empty(t, rec.Header().Get("Trailer"), "must not rely on a trailer")
}

func TestReviewHandlers_GetPatch_UntruncatedSendsNoTruncationValue(t *testing.T) {
	rec := getPatch(newRouter(), httptest.NewRecorder(), "?path=a.go&maxLines=100")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Result().Trailer.Get("X-Crowbar-Diff-Truncated"))
}

func TestReviewHandlers_GetPatch_UnlimitedDeclaresNoTrailer(t *testing.T) {
	rec := getPatch(newRouter(), httptest.NewRecorder(), "?path=a.go&maxLines=0")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Trailer"))
}

func TestReviewHandlers_GetPatch_MaxLines(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"absent defaults to 20000", "?path=a.go", 20000},
		{"explicit cap is honoured", "?path=a.go&maxLines=42", 42},
		{"zero means unlimited", "?path=a.go&maxLines=0", 0},
		{"negative means unlimited", "?path=a.go&maxLines=-1", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := -999
			uc := stubUsecase{
				patch: func(_ context.Context, _, _, _ string, maxLines int, _ io.Writer) (int, bool, error) {
					got = maxLines
					return 0, false, nil
				},
			}
			rec := getPatch(routerFor(uc), httptest.NewRecorder(), tc.query)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReviewHandlers_GetPatch_MissingPathIsBadRequest(t *testing.T) {
	for _, query := range []string{"", "?path=", "?maxLines=10"} {
		called := false
		uc := stubUsecase{
			patch: func(_ context.Context, _, _, _ string, _ int, _ io.Writer) (int, bool, error) {
				called = true
				return 0, false, nil
			},
		}
		rec := getPatch(routerFor(uc), httptest.NewRecorder(), query)

		assert.Equalf(t, http.StatusBadRequest, rec.Code, "query %q", query)
		assert.Falsef(t, called, "query %q must not reach the usecase", query)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	}
}

func TestReviewHandlers_GetPatch_EngineLevelEmptyPathGuardIsA400(t *testing.T) {
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, _ io.Writer) (int, bool, error) {
			return 0, false, fmt.Errorf("branch review: patch: %w: path is required", problem.ErrInvalidArgument)
		},
	}
	rec := getPatch(routerFor(uc), httptest.NewRecorder(), "?path=a.go")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReviewHandlers_GetPatch_InvalidMaxLinesIsBadRequest(t *testing.T) {
	rec := getPatch(newRouter(), httptest.NewRecorder(), "?path=a.go&maxLines=lots")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReviewHandlers_GetPatch_FailureBeforeAnyWriteIsAJSONEnvelope(t *testing.T) {
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, _ io.Writer) (int, bool, error) {
			return 0, false, errors.New("git boom")
		},
	}
	rec := getPatch(routerFor(uc), httptest.NewRecorder(), "?path=a.go")

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.False(t, env.Success)
	assert.Contains(t, env.Error, "git boom")
	assert.Empty(t, rec.Header().Get("Trailer"))
}

func TestReviewHandlers_GetPatch_NotFoundBeforeAnyWrite(t *testing.T) {
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, _ io.Writer) (int, bool, error) {
			return 0, false, problem.ErrNotFound
		},
	}
	rec := getPatch(routerFor(uc), httptest.NewRecorder(), "?path=a.go")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestReviewHandlers_GetPatch_FailureMidStreamKeepsWhatWasWritten(t *testing.T) {
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			_, _ = io.WriteString(w, "@@ -1,1 +1,1 @@\n")
			return 1, false, errors.New("git died")
		},
	}
	rec := getPatch(routerFor(uc), httptest.NewRecorder(), "?path=a.go&maxLines=0")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "@@ -1,1 +1,1 @@\n", rec.Body.String())
}

func TestReviewHandlers_GetPatch_ForwardsPathAndWorkspace(t *testing.T) {
	var gotWs, gotPath string
	uc := stubUsecase{
		patch: func(_ context.Context, wsID, _, path string, _ int, _ io.Writer) (int, bool, error) {
			gotWs, gotPath = wsID, path
			return 0, false, nil
		},
	}
	getPatch(routerFor(uc), httptest.NewRecorder(), "?path=dir%2Fwe%2Aird%5B1%5D.txt")

	assert.Equal(t, "chat1", gotWs)
	assert.Equal(t, "dir/we*ird[1].txt", gotPath)
}

func TestReviewHandlers_GetPatch_OversizedCapStreamsInsteadOfBuffering(t *testing.T) {
	rec := httptest.NewRecorder()

	const lines = 4
	const line = "+streamed line\n"
	var seen []int
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			for range lines {
				if _, err := io.WriteString(w, line); err != nil {
					return 0, false, err
				}
				seen = append(seen, rec.Body.Len())
			}
			return lines, true, nil
		},
	}

	getPatch(routerFor(uc), rec, "?path=a.go&maxLines=500000")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []int{len(line), 2 * len(line), 3 * len(line), 4 * len(line)}, seen,
		"an oversized cap must still stream")
	assert.Empty(t, rec.Result().Header.Get("X-Crowbar-Diff-Truncated"),
		"the streaming path cannot report truncation in a leading header")
}

func TestReviewHandlers_GetPatch_CappedRequestBuffersToLearnTruncation(t *testing.T) {
	rec := httptest.NewRecorder()

	var seen []int
	uc := stubUsecase{
		patch: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			for range 3 {
				if _, err := io.WriteString(w, "+x\n"); err != nil {
					return 0, false, err
				}
				seen = append(seen, rec.Body.Len())
			}
			return 3, true, nil
		},
	}

	getPatch(routerFor(uc), rec, "?path=a.go&maxLines=10")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []int{0, 0, 0}, seen, "capped requests are held, not streamed")
	assert.Equal(t, "+x\n+x\n+x\n", rec.Body.String(), "the whole body still arrives")
	assert.Equal(t, "true", rec.Result().Header.Get("X-Crowbar-Diff-Truncated"))
}
