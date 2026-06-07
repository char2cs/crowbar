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

func TestDiffWorkingTreeUnstaged(
	t *testing.T,
) {
	git := &fakeGit{
		diff: []gitdomain.FileDiff{
			{FilePath: "a.go"},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, git.diffStaged)
	assert.Empty(t, git.gotCommit)
	var body struct {
		Data []struct {
			FilePath string               `json:"file_path"`
			Lines    []gitdomain.DiffLine `json:"lines"`
			Hunks    []gitdomain.Hunk     `json:"hunks"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "a.go", body.Data[0].FilePath)
	assert.NotNil(t, body.Data[0].Lines)
	assert.NotNil(t, body.Data[0].Hunks)
}

func TestDiffWorkingTreeStaged(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff?staged=true")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, git.diffStaged)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestDiffFiltersByPath(
	t *testing.T,
) {
	git := &fakeGit{
		diff: []gitdomain.FileDiff{
			{FilePath: "a.go"},
			{FilePath: "b.go"},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff?path=b.go")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []struct {
			FilePath string `json:"file_path"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "b.go", body.Data[0].FilePath)
}

func TestDiffCommit(
	t *testing.T,
) {
	git := &fakeGit{
		commitDiff: gitdomain.MultiFileDiff{
			CommitHash: "deadbeef",
			Files:      []gitdomain.FileDiff{{FilePath: "a.go"}},
			TotalFiles: 1,
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff?commit=deadbeef")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "deadbeef", git.gotCommit)
	var body struct {
		Data struct {
			CommitHash string `json:"commitHash"`
			TotalFiles int    `json:"totalFiles"`
			Files      []struct {
				FilePath string               `json:"file_path"`
				Lines    []gitdomain.DiffLine `json:"lines"`
			} `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "deadbeef", body.Data.CommitHash)
	assert.Equal(t, 1, body.Data.TotalFiles)
	require.Len(t, body.Data.Files, 1)
	assert.NotNil(t, body.Data.Files[0].Lines)
}

func TestDiffCommitFiltersByPath(
	t *testing.T,
) {
	git := &fakeGit{
		commitDiff: gitdomain.MultiFileDiff{
			CommitHash: "deadbeef",
			Files: []gitdomain.FileDiff{
				{FilePath: "a.go", Additions: 3, Deletions: 1},
				{FilePath: "b.go", Additions: 5, Deletions: 2},
			},
			TotalFiles:     2,
			TotalAdditions: 8,
			TotalDeletions: 3,
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff?commit=deadbeef&path=b.go")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			TotalFiles     int `json:"totalFiles"`
			TotalAdditions int `json:"totalAdditions"`
			TotalDeletions int `json:"totalDeletions"`
			Files          []struct {
				FilePath string `json:"file_path"`
			} `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Files, 1)
	assert.Equal(t, "b.go", body.Data.Files[0].FilePath)
	assert.Equal(t, 1, body.Data.TotalFiles)
	assert.Equal(t, 5, body.Data.TotalAdditions)
	assert.Equal(t, 2, body.Data.TotalDeletions)
}

func TestDiffBadStaged(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/diff?staged=maybe")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDiffWorkingTreeError(
	t *testing.T,
) {
	git := &fakeGit{diffErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDiffCommitError(
	t *testing.T,
) {
	git := &fakeGit{commitErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/diff?commit=x")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
