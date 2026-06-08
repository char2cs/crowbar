package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitWriteHandlers_MissingFields(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/stage",
		map[string]any{"path": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/stage-hunk",
		map[string]any{"path": "a.go", "hunkId": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/unstage",
		map[string]any{"path": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/unstage-hunk",
		map[string]any{"path": "a.go", "hunkId": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/discard",
		map[string]any{"path": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/commit",
		map[string]any{"message": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/branches",
		map[string]any{"name": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPatch, ws+"/branches",
		map[string]any{"from": "old", "to": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodDelete, ws+"/branches", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/switch",
		map[string]any{"branch": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodDelete, ws+"/stash", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/reset",
		map[string]any{"mode": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/merge",
		map[string]any{"branch": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/rebase",
		map[string]any{"branch": ""}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/resolve-hunk",
		map[string]any{"path": "a.go", "choice": ""}).Code)
}
