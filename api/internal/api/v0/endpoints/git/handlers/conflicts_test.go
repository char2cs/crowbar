package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestConflictsSuccess(
	t *testing.T,
) {
	git := &fakeGit{
		conflictFiles: []string{"a.txt", "b.txt"},
		hunks: []gitdomain.ConflictHunk{
			{
				ID:         "h1",
				StartLine:  1,
				EndLine:    4,
				Ours:       "x",
				Theirs:     "y",
				Base:       "z",
				Resolution: gitdomain.ConflictResolutionUnresolved,
			},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/conflicts")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []struct {
			Path  string `json:"path"`
			Hunks []struct {
				ID         string `json:"id"`
				StartLine  int    `json:"startLine"`
				Base       string `json:"base"`
				Resolution string `json:"resolution"`
			} `json:"hunks"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	assert.Equal(t, "a.txt", body.Data[0].Path)
	require.Len(t, body.Data[0].Hunks, 1)
	assert.Equal(t, "h1", body.Data[0].Hunks[0].ID)
	assert.Equal(t, "unresolved", body.Data[0].Hunks[0].Resolution)
	assert.Equal(t, []string{"a.txt", "b.txt"}, git.hunksForPath)
}

func TestConflictsEmptyNonNil(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/conflicts")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestConflictsHunksEmptyNonNil(
	t *testing.T,
) {
	git := &fakeGit{conflictFiles: []string{"a.txt"}}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/conflicts")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"hunks":[]`)
}

func TestConflictsFilesError(
	t *testing.T,
) {
	git := &fakeGit{conflictErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/conflicts")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestConflictsHunksError(
	t *testing.T,
) {
	git := &fakeGit{
		conflictFiles: []string{"a.txt"},
		hunksErr:      errors.New("boom"),
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/conflicts")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestResolveConflictSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	body := `{"path":"a.txt","conflictHunkId":"h1","resolution":"custom","resolvedContent":"merged"}`
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"w1"`)
	assert.Equal(t, "w1", git.resolveCall.wsID)
	assert.Equal(t, "a.txt", git.resolveCall.path)
	assert.Equal(t, "h1", git.resolveCall.hunkID)
	assert.Equal(t, gitdomain.ConflictResolutionCustom, git.resolveCall.resolution)
	assert.Equal(t, "merged", git.resolveCall.content)
}

func TestResolveConflictNonCustomSuccess(
	t *testing.T,
) {
	git := &fakeGit{}
	body := `{"path":"a.txt","conflictHunkId":"h1","resolution":"ours"}`
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, gitdomain.ConflictResolutionOurs, git.resolveCall.resolution)
}

func TestResolveConflictBadJSON(
	t *testing.T,
) {
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", "{")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResolveConflictMissingPath(
	t *testing.T,
) {
	body := `{"conflictHunkId":"h1","resolution":"ours"}`
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "path is required")
}

func TestResolveConflictMissingHunkID(
	t *testing.T,
) {
	body := `{"path":"a.txt","resolution":"ours"}`
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "conflictHunkId is required")
}

func TestResolveConflictInvalidResolution(
	t *testing.T,
) {
	body := `{"path":"a.txt","conflictHunkId":"h1","resolution":"nope"}`
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "resolution must be one of")
}

func TestResolveConflictCustomWithoutContent(
	t *testing.T,
) {
	body := `{"path":"a.txt","conflictHunkId":"h1","resolution":"custom"}`
	rec := send(newRouter(&fakeGit{}), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "resolvedContent is required")
}

func TestResolveConflictError(
	t *testing.T,
) {
	git := &fakeGit{writeErr: errors.New("boom")}
	body := `{"path":"a.txt","conflictHunkId":"h1","resolution":"ours"}`
	rec := send(newRouter(git), http.MethodPost, "/v0/workspaces/w1/git/conflicts/resolve", body)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
