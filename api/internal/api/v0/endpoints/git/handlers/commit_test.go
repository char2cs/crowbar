package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/commit", `{"subject":"feat","body":"detail"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"w1"`)
	require.Len(t, git.calls, 1)
	assert.Equal(t, "Commit", git.calls[0].method)
	assert.Equal(t, []string{"feat", "detail"}, git.calls[0].args)
}

func TestCommitMissingSubject(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/commit", `{"body":"x"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, git.calls)
}

func TestCommitMalformedJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/commit", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCommitUsecaseError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/commit", `{"subject":"x"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
