package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagePathsDispatchesStageFile(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stage", `{"paths":["a.go","b.go"]}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"w1"`)
	require.Len(t, git.calls, 2)
	assert.Equal(t, "StageFile", git.calls[0].method)
	assert.Equal(t, "w1", git.calls[0].wsID)
	assert.Equal(t, []string{"a.go"}, git.calls[0].args)
	assert.Equal(t, "StageFile", git.calls[1].method)
	assert.Equal(t, []string{"b.go"}, git.calls[1].args)
}

func TestStageHunkDispatchesStageHunk(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stage", `{"path":"a.go","hunkId":"h1"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "StageHunk", git.calls[0].method)
	assert.Equal(t, []string{"a.go", "h1"}, git.calls[0].args)
}

func TestStageNeitherPathsNorHunk(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stage", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestStageHunkMissingPath(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/stage", `{"hunkId":"h1"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStageMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/stage", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStageUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stage", `{"paths":["a.go"]}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestUnstagePathsDispatchesUnstageFile(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/unstage", `{"paths":["a.go"]}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "UnstageFile", git.calls[0].method)
	assert.Equal(t, []string{"a.go"}, git.calls[0].args)
}

func TestUnstageHunkDispatchesUnstageHunk(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/unstage", `{"path":"a.go","hunkId":"h2"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "UnstageHunk", git.calls[0].method)
	assert.Equal(t, []string{"a.go", "h2"}, git.calls[0].args)
}

func TestUnstageNeitherPathsNorHunk(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/unstage", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUnstageMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/unstage", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUnstageUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/unstage", `{"path":"a.go","hunkId":"h2"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestUnstagePathsUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/unstage", `{"paths":["a.go"]}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDiscardDispatchesDiscard(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/discard", `{"paths":["a.go","b.go"]}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 2)
	assert.Equal(t, "Discard", git.calls[0].method)
	assert.Equal(t, []string{"a.go"}, git.calls[0].args)
	assert.Equal(t, []string{"b.go"}, git.calls[1].args)
}

func TestDiscardEmptyPaths(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/discard", `{"paths":[]}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestDiscardMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/discard", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDiscardUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/discard", `{"paths":["a.go"]}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
