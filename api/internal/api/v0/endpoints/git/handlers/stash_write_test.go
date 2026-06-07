package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStashPushSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash", `{"message":"wip"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "StashPush", git.calls[0].method)
	assert.Equal(t, []string{"wip"}, git.calls[0].args)
}

func TestStashPushEmptyMessage(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash", `{}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, []string{""}, git.calls[0].args)
}

func TestStashPushMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/stash", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStashPushUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash", `{}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestStashApply(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash/2", `{"mode":"apply"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "StashApply", git.calls[0].method)
	assert.Equal(t, []string{"2"}, git.calls[0].args)
}

func TestStashPop(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash/0", `{"mode":"pop"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "StashPop", git.calls[0].method)
	assert.Equal(t, []string{"0"}, git.calls[0].args)
}

func TestStashRestoreInvalidMode(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash/0", `{"mode":"drop"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestStashRestoreMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/stash/0", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStashRestoreUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/stash/0", `{"mode":"pop"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestStashDropSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodDelete, "/v0/workspaces/w1/git/stash/3", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "StashDrop", git.calls[0].method)
	assert.Equal(t, []string{"3"}, git.calls[0].args)
}

func TestStashDropUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodDelete, "/v0/workspaces/w1/git/stash/3", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
