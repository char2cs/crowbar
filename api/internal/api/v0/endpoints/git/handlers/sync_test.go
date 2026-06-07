package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/push", `{}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Push", git.calls[0].method)
	assert.Equal(t, "w1", git.calls[0].wsID)
}

func TestPushUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/push", `{}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPullMerge(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/pull", `{"mode":"merge"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Pull", git.calls[0].method)
	assert.Equal(t, []string{"merge"}, git.calls[0].args)
}

func TestPullRebase(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/pull", `{"mode":"rebase"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"rebase"}, git.calls[0].args)
}

func TestPullInvalidMode(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/pull", `{"mode":"squash"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestPullMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/pull", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPullUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/pull", `{"mode":"merge"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestFetchSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/fetch", `{}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Fetch", git.calls[0].method)
}

func TestFetchUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/fetch", `{}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
