package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// stubUsecase answers the whole ReviewUsecase surface with fixed data, and lets
// a test replace any single windowed-diff method with a probe. Nil fields keep
// the fixed answer, so a test only spells out the method it is about.
type stubUsecase struct {
	files     func(ctx context.Context, wsID, commit string) ([]gitdomain.ReviewFileSummary, error)
	outline   func(ctx context.Context, wsID, commit string) ([]gitdomain.FileOutline, error)
	patch     func(ctx context.Context, wsID, commit, path string, maxLines int, w io.Writer) (int, bool, error)                    //nolint:lll // one field per stub method; wrapping the signature hides which method it stands in for.
	searchFn  func(ctx context.Context, wsID, commit, query string, opts gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) //nolint:lll // ditto.
	searchErr error
}

func (stubUsecase) Get(
	_ context.Context,
	_ string,
) (domain.BranchReview, error) {
	return domain.BranchReview{}, nil
}

func (s stubUsecase) GetFiles(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.ReviewFileSummary, error) {
	if s.files != nil {
		return s.files(ctx, wsID, commit)
	}
	return []gitdomain.ReviewFileSummary{
		{Path: "committed.go", Status: gitdomain.GitFileStatusModified, Additions: 3, Deletions: 1},
		{Path: "wip.go", Status: gitdomain.GitFileStatusAdded, Additions: 2, Uncommitted: true, Staged: true},
	}, nil
}

func (stubUsecase) SetMergeStrategy(
	_ context.Context,
	_ string,
	_ gitdomain.MergeStrategy,
) error {
	return nil
}

func (s stubUsecase) GetOutline(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.FileOutline, error) {
	if s.outline != nil {
		return s.outline(ctx, wsID, commit)
	}
	return []gitdomain.FileOutline{
		{Path: "a.go", Hunks: []gitdomain.HunkShape{{OldStart: 1, OldLines: 4, NewStart: 1, NewLines: 6}}},
		{Path: "b.bin", IsBinary: true},
	}, nil
}

func (s stubUsecase) GetPatch(
	ctx context.Context,
	wsID string,
	commit string,
	path string,
	maxLines int,
	w io.Writer,
) (int, bool, error) {
	if s.patch != nil {
		return s.patch(ctx, wsID, commit, path, maxLines, w)
	}
	n, err := io.WriteString(w, "diff --git a/a.go b/a.go\n")
	return n, false, err
}

func (s stubUsecase) SearchDiff(
	ctx context.Context,
	wsID string,
	commit string,
	query string,
	opts gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	if s.searchErr != nil {
		return nil, false, s.searchErr
	}
	if s.searchFn != nil {
		return s.searchFn(ctx, wsID, commit, query, opts)
	}
	return []gitdomain.SearchHit{
		{Path: "a.go", Side: gitdomain.SearchSideNew, LineNumber: 12, Preview: "todo"},
	}, false, nil
}

func routerFor(uc handlers.ReviewUsecase) *gin.Engine {
	r := gin.New()
	h := handlers.New(uc)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/review", h.Get)
	rg.GET("/workspaces/:wsId/review/files", h.GetFiles)
	rg.GET("/workspaces/:wsId/review/outline", h.GetOutline)
	rg.GET("/workspaces/:wsId/review/patch", h.GetPatch)
	rg.GET("/workspaces/:wsId/review/search", h.SearchDiff)
	rg.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
	return r
}

func newRouter() *gin.Engine {
	return routerFor(stubUsecase{})
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

func TestReviewHandlers_GetFiles(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/review/files", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Files []gitdomain.ReviewFileSummary `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Len(t, env.Data.Files, 2)
	assert.Equal(t, "committed.go", env.Data.Files[0].Path)
	assert.Equal(t, 3, env.Data.Files[0].Additions)
	assert.False(t, env.Data.Files[0].Uncommitted)
	assert.True(t, env.Data.Files[1].Uncommitted)
	assert.True(t, env.Data.Files[1].Staged)
}
