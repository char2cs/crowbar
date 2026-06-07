package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBranchSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/branches", `{"name":"feat","source":"main"}`)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"w1"`)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "CreateBranch", git.calls[0].method)
	assert.Equal(t, []string{"feat", "main"}, git.calls[0].args)
	assert.False(t, git.calls[0].boolArg)
}

func TestCreateBranchMissingName(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/branches", `{"source":"main"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestCreateBranchMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/branches", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateBranchUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/branches", `{"name":"feat"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRenameBranchSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPatch, "/v0/workspaces/w1/git/branches/old", `{"newName":"new"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "RenameBranch", git.calls[0].method)
	assert.Equal(t, []string{"old", "new"}, git.calls[0].args)
}

func TestRenameBranchURLEncodedSlash(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPatch, "/v0/workspaces/w1/git/branches/feature%2Fauth", `{"newName":"feature/login"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, []string{"feature/auth", "feature/login"}, git.calls[0].args)
}

func TestRenameBranchMissingNewName(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPatch, "/v0/workspaces/w1/git/branches/old", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestRenameBranchMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPatch, "/v0/workspaces/w1/git/branches/old", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameBranchUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPatch, "/v0/workspaces/w1/git/branches/old", `{"newName":"new"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDeleteBranchSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodDelete, "/v0/workspaces/w1/git/branches/feature%2Fauth", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "DeleteBranch", git.calls[0].method)
	assert.Equal(t, []string{"feature/auth"}, git.calls[0].args)
}

func TestDeleteBranchUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodDelete, "/v0/workspaces/w1/git/branches/old", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCheckoutSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/checkout", `{"branch":"main"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "SwitchBranch", git.calls[0].method)
	assert.Equal(t, []string{"main"}, git.calls[0].args)
}

func TestCheckoutMissingBranch(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/checkout", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestCheckoutMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/checkout", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCheckoutUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/checkout", `{"branch":"main"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
