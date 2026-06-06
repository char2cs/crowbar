package v0_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// reviewRouter builds the v0 router for review handler tests.
func reviewRouter(
	t *testing.T,
	tc testContainers,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	return r
}

// gitRun runs a git command in dir, failing the test on error.
func reviewGitRun(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// seedWorkspace creates a real temp git repo with a feature branch and seeds the
// workspace + repository rows so the review GET can run a real range diff.
func seedReviewWorkspace(
	t *testing.T,
	tc testContainers,
) domain.Workspace {
	t.Helper()
	dir := t.TempDir()
	reviewGitRun(t, dir, "init", "-b", "main")
	reviewGitRun(t, dir, "config", "user.email", "test@test.com")
	reviewGitRun(t, dir, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.go"), []byte("package main\n"), 0o600))
	reviewGitRun(t, dir, "add", "base.go")
	reviewGitRun(t, dir, "commit", "-m", "base")
	reviewGitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package main\n\nfunc f() {}\n"), 0o600))
	reviewGitRun(t, dir, "add", "feature.go")
	reviewGitRun(t, dir, "commit", "-m", "feature")

	ctx := context.Background()
	require.NoError(t, tc.app.GORM.Repositories.Save(ctx, domain.Repository{
		ID:            "repo1",
		ProjectID:     "proj1",
		DefaultBranch: "main",
	}))
	ws, err := tc.app.Repositories.Workspace.Create(ctx, workspace.CreateInput{
		ID:           "ws1",
		RepoID:       "repo1",
		ProjectID:    "proj1",
		Branch:       "feature",
		WorktreePath: dir,
	}, time.Now())
	require.NoError(t, err)
	return ws
}

func doJSON(
	t *testing.T,
	r *gin.Engine,
	method string,
	path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestReview_Get_ReturnsComposite(t *testing.T) {
	tc := newApp(t)
	seedReviewWorkspace(t, tc)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodGet, "/v0/workspaces/ws1/review", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Contains(t, got, "mergeStrategy")
	assert.Contains(t, got, "diff")
	assert.Contains(t, got, "threads")
	assert.Contains(t, got, "conversations")
}

func TestReview_Get_UnknownWorkspace_404(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodGet, "/v0/workspaces/nope/review", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestReview_SetMergeStrategy_ThenGetShowsSquash(t *testing.T) {
	tc := newApp(t)
	seedReviewWorkspace(t, tc)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodPatch, "/v0/workspaces/ws1/review",
		map[string]any{"mergeStrategy": string(gitdomain.MergeStrategySquash)})
	require.Equal(t, http.StatusOK, w.Code)

	g := doJSON(t, r, http.MethodGet, "/v0/workspaces/ws1/review", nil)
	require.Equal(t, http.StatusOK, g.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(g.Body.Bytes(), &got))
	assert.Equal(t, string(gitdomain.MergeStrategySquash), got["mergeStrategy"])
}

func TestReview_SetMergeStrategy_BadBody_400(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	req := httptest.NewRequest(http.MethodPatch, "/v0/workspaces/ws1/review", bytes.NewReader([]byte("{")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReview_SetMergeStrategy_UnknownWorkspace_404(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodPatch, "/v0/workspaces/nope/review",
		map[string]any{"mergeStrategy": string(gitdomain.MergeStrategySquash)})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestReview_OpenThread_AndReply(t *testing.T) {
	tc := newApp(t)
	seedReviewWorkspace(t, tc)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"filePath":   "feature.go",
		"lineNumber": 3,
		"side":       string(domain.ReviewSideRight),
		"body":       "please rename",
	})
	require.Equal(t, http.StatusCreated, w.Code)

	var thread domain.ReviewThread
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &thread))
	require.NotEmpty(t, thread.ID)
	require.Len(t, thread.Messages, 1)

	rep := doJSON(t, r, http.MethodPost,
		"/v0/workspaces/ws1/review/threads/"+thread.ID+"/reply",
		map[string]any{"body": "done"})
	require.Equal(t, http.StatusOK, rep.Code)
	var replied domain.ReviewThread
	require.NoError(t, json.Unmarshal(rep.Body.Bytes(), &replied))
	assert.Len(t, replied.Messages, 2)
}

func TestReview_OpenThread_BadBody_400(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/ws1/review/threads", bytes.NewReader([]byte("{")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReview_Reply_BadBody_400(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/ws1/review/threads/t1/reply", bytes.NewReader([]byte("{")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReview_Reply_UnknownThread_404(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads/nope/reply",
		map[string]any{"body": "hi"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestReview_SetThreadResolved_TrueThenFalse(t *testing.T) {
	tc := newApp(t)
	seedReviewWorkspace(t, tc)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"filePath":   "feature.go",
		"lineNumber": 1,
		"side":       string(domain.ReviewSideRight),
		"body":       "comment",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var thread domain.ReviewThread
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &thread))

	res := doJSON(t, r, http.MethodPatch,
		"/v0/workspaces/ws1/review/threads/"+thread.ID,
		map[string]any{"isResolved": true})
	require.Equal(t, http.StatusOK, res.Code)
	var resolved domain.ReviewThread
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &resolved))
	assert.True(t, resolved.IsResolved())

	reopen := doJSON(t, r, http.MethodPatch,
		"/v0/workspaces/ws1/review/threads/"+thread.ID,
		map[string]any{"isResolved": false})
	require.Equal(t, http.StatusOK, reopen.Code)
	var reopened domain.ReviewThread
	require.NoError(t, json.Unmarshal(reopen.Body.Bytes(), &reopened))
	assert.False(t, reopened.IsResolved())
}

func TestReview_SetThreadResolved_BadBody_400(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	req := httptest.NewRequest(http.MethodPatch, "/v0/workspaces/ws1/review/threads/t1", bytes.NewReader([]byte("{")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReview_SetThreadResolved_UnknownThread_404(t *testing.T) {
	tc := newApp(t)
	r := reviewRouter(t, tc)

	w := doJSON(t, r, http.MethodPatch, "/v0/workspaces/ws1/review/threads/nope",
		map[string]any{"isResolved": true})
	assert.Equal(t, http.StatusNotFound, w.Code)
}
