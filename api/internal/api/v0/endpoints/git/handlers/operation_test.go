package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationContinueSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/operation/continue", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"w1"`)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "OperationContinue", git.calls[0].method)
	assert.Equal(t, "w1", git.calls[0].wsID)
}

func TestOperationContinueError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/operation/continue", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestOperationAbortSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/operation/abort", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"w1"`)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "OperationAbort", git.calls[0].method)
	assert.Equal(t, "w1", git.calls[0].wsID)
}

func TestOperationAbortError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/operation/abort", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
