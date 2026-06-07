package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/reset", `{"mode":"hard","commit":"abc123"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Reset", git.calls[0].method)
	assert.Equal(t, []string{"hard", "abc123"}, git.calls[0].args)
}

func TestResetSoftAndMixed(
	t *testing.T,
) {
	for _, mode := range []string{"soft", "mixed"} {
		git := &fakeGit{}
		rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/reset", `{"mode":"`+mode+`","commit":"c"}`)
		assert.Equal(t, http.StatusOK, rec.Code, mode)
		require.Len(t, git.calls, 1)
		assert.Equal(t, []string{mode, "c"}, git.calls[0].args)
	}
}

func TestResetInvalidMode(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/reset", `{"mode":"keep","commit":"c"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestResetMissingCommit(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/reset", `{"mode":"hard"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestResetMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/reset", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResetUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/reset", `{"mode":"hard","commit":"c"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestMergeSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/merge", `{"branch":"feat"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Merge", git.calls[0].method)
	assert.Equal(t, []string{"feat"}, git.calls[0].args)
}

func TestMergeMissingBranch(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/merge", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestMergeMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/merge", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMergeUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/merge", `{"branch":"feat"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRebaseSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/rebase", `{"onto":"main"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Rebase", git.calls[0].method)
	assert.Equal(t, []string{"main"}, git.calls[0].args)
}

func TestRebaseMissingOnto(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/rebase", `{}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestRebaseMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/rebase", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRebaseUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/rebase", `{"onto":"main"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
