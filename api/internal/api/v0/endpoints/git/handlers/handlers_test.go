package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubGit struct{}

func (stubGit) Status(_ context.Context, _ string) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (stubGit) Diff(_ context.Context, _ string, _ bool) ([]gitdomain.FileDiff, error) {
	return nil, nil
}

func (stubGit) Log(_ context.Context, _ string, _ int, _ int) ([]gitdomain.Commit, error) {
	return nil, nil
}

func (stubGit) Blame(_ context.Context, _ string, _ string) ([]gitdomain.BlameEntry, error) {
	return nil, nil
}

func (stubGit) Branches(_ context.Context, _ string) ([]gitdomain.Branch, error) {
	return nil, nil
}

func (stubGit) Stashes(_ context.Context, _ string) ([]gitdomain.Stash, error) {
	return nil, nil
}

func (stubGit) ConflictedFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (stubGit) ConflictHunks(_ context.Context, _ string, _ string) ([]gitdomain.ConflictHunk, error) {
	return nil, nil
}

func (stubGit) CommitDiff(_ context.Context, _ string, _ string) (gitdomain.MultiFileDiff, error) {
	return gitdomain.MultiFileDiff{}, nil
}

func (stubGit) StageFile(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) StageHunk(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) UnstageFile(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) UnstageHunk(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) Discard(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) Commit(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Push(_ context.Context, _ string, _ time.Time) error           { return nil }
func (stubGit) Fetch(_ context.Context, _ string, _ time.Time) error          { return nil }
func (stubGit) Pull(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) CreateBranch(_ context.Context, _ string, _ string, _ string, _ bool, _ time.Time) error {
	return nil
}

func (stubGit) RenameBranch(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) DeleteBranch(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) SwitchBranch(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) StashPush(_ context.Context, _ string, _ string, _ time.Time) error    { return nil }
func (stubGit) StashApply(_ context.Context, _ string, _ string, _ time.Time) error   { return nil }
func (stubGit) StashPop(_ context.Context, _ string, _ string, _ time.Time) error     { return nil }
func (stubGit) StashDrop(_ context.Context, _ string, _ string, _ time.Time) error    { return nil }
func (stubGit) Reset(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Merge(_ context.Context, _ string, _ string, _ time.Time) error  { return nil }
func (stubGit) Rebase(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) ResolveHunk(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ gitdomain.ConflictResolution,
	_ string,
	_ time.Time,
) error {
	return nil
}
func (stubGit) OperationContinue(_ context.Context, _ string, _ time.Time) error { return nil }
func (stubGit) OperationAbort(_ context.Context, _ string, _ time.Time) error    { return nil }

// fakeLastErrors records the workspace error sink calls so the slow-git async
// failure path can be asserted without a sleep.
type fakeLastErrors struct {
	gotID  string
	gotMsg string
	called chan struct{}
}

func (f *fakeLastErrors) SetLastError(
	_ context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	f.gotID = id
	f.gotMsg = message
	if f.called != nil {
		f.called <- struct{}{}
	}
	return domain.Workspace{ID: id, LastError: message}, nil
}

func newRouter() *gin.Engine {
	return newRouterWith(stubGit{})
}

func newRouterWith(g handlers.Git) *gin.Engine {
	return newRouterWithErrors(g, &fakeLastErrors{})
}

func newRouterWithErrors(
	g handlers.Git,
	lastErrors handlers.LastErrorSetter,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(g, lastErrors)
	rg := r.Group("/v0/workspaces/:wsId/git")
	rg.GET("/status", h.Status)
	rg.GET("/diff", h.Diff)
	rg.GET("/log", h.Log)
	rg.GET("/blame", h.Blame)
	rg.GET("/branches", h.Branches)
	rg.GET("/stashes", h.Stashes)
	rg.GET("/conflicts", h.Conflicts)
	rg.GET("/conflict-hunks", h.ConflictHunks)
	rg.GET("/commit-diff", h.CommitDiff)
	rg.POST("/stage", h.Stage)
	rg.POST("/stage-hunk", h.StageHunk)
	rg.POST("/unstage", h.Unstage)
	rg.POST("/unstage-hunk", h.UnstageHunk)
	rg.POST("/discard", h.Discard)
	rg.POST("/commit", h.Commit)
	rg.POST("/push", h.Push)
	rg.POST("/fetch", h.Fetch)
	rg.POST("/pull", h.Pull)
	rg.POST("/branches", h.CreateBranch)
	rg.PATCH("/branches", h.RenameBranch)
	rg.DELETE("/branches", h.DeleteBranch)
	rg.POST("/switch", h.Switch)
	rg.POST("/stash", h.StashPush)
	rg.POST("/stash-apply", h.StashApply)
	rg.POST("/stash-pop", h.StashPop)
	rg.DELETE("/stash", h.StashDrop)
	rg.POST("/reset", h.Reset)
	rg.POST("/merge", h.Merge)
	rg.POST("/rebase", h.Rebase)
	rg.POST("/resolve-hunk", h.ResolveHunk)
	rg.POST("/operation/continue", h.OperationContinue)
	rg.POST("/operation/abort", h.OperationAbort)
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

const ws = "/v0/workspaces/ws1/git"

func TestGitReadHandlers(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/status", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/diff", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/log", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/blame?path=main.go", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/branches", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/stashes", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/conflicts", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/conflict-hunks?path=main.go", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, ws+"/commit-diff?sha=abc123", nil).Code)
}

func TestGitReadHandlers_MissingParams(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodGet, ws+"/blame", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodGet, ws+"/conflict-hunks", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodGet, ws+"/commit-diff", nil).Code)
}

func TestGitWriteHandlers(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/stage",
		map[string]any{"paths": []string{"a.go"}}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/stage-hunk",
		map[string]any{"path": "a.go", "hunkId": "h1"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/unstage",
		map[string]any{"paths": []string{"a.go"}}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/unstage-hunk",
		map[string]any{"path": "a.go", "hunkId": "h1"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/discard",
		map[string]any{"paths": []string{"a.go"}}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/commit",
		map[string]any{"subject": "feat: add", "body": "details"}).Code)
	assert.Equal(t, http.StatusCreated, do(r, http.MethodPost, ws+"/branches",
		map[string]any{"name": "feat/x"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPatch, ws+"/branches",
		map[string]any{"from": "old", "to": "new"}).Code)
	assert.Equal(t, http.StatusNoContent, do(r, http.MethodDelete, ws+"/branches?name=feat/x", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/switch",
		map[string]any{"branch": "main"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/stash", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/stash-apply",
		map[string]any{"index": 0}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/stash-pop",
		map[string]any{"index": 0}).Code)
	assert.Equal(t, http.StatusNoContent, do(r, http.MethodDelete, ws+"/stash?index=0", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/reset",
		map[string]any{"mode": "soft"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/resolve-hunk",
		map[string]any{"path": "a.go", "hunkIndex": 0, "choice": "ours"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/operation/continue", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, ws+"/operation/abort", nil).Code)
}
