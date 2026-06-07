package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestStatusSuccess(
	t *testing.T,
) {
	git := &fakeGit{
		status: gitdomain.GitStatus{
			Branch: "main",
			Ahead:  1,
			Behind: 2,
			Files: []gitdomain.GitFile{
				{Path: "a.go", Status: gitdomain.GitFileStatusModified, Staged: true},
			},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/status")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Branch string `json:"branch"`
			Ahead  int    `json:"ahead"`
			Behind int    `json:"behind"`
			Files  []struct {
				Path   string `json:"path"`
				Status string `json:"status"`
				Staged bool   `json:"staged"`
			} `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "main", body.Data.Branch)
	assert.Equal(t, 1, body.Data.Ahead)
	assert.Equal(t, 2, body.Data.Behind)
	require.Len(t, body.Data.Files, 1)
	assert.Equal(t, "a.go", body.Data.Files[0].Path)
	assert.Equal(t, "modified", body.Data.Files[0].Status)
	assert.True(t, body.Data.Files[0].Staged)
}

func TestStatusEmptyFilesNonNil(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/status")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"files":[]`)
}

func TestStatusNotFound(
	t *testing.T,
) {
	git := &fakeGit{statusErr: apperr.ErrNotFound}
	rec := do(newRouter(git), "/v0/workspaces/nope/git/status")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStatusError(
	t *testing.T,
) {
	git := &fakeGit{statusErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/status")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
