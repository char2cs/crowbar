package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestListSuccess(
	t *testing.T,
) {
	reader := &fakeReader{
		list: []domain.Workspace{
			{ID: "w1", RepoID: "r1", ProjectID: "p1"},
		},
	}
	rec := do(newRouter(reader, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p1/repos/r1/workspaces", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.Len(t, body.Data, 1)
	assert.Equal(t, "w1", body.Data[0].ID)
}

func TestListEmptyNonNil(
	t *testing.T,
) {
	rec := do(newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p1/repos/r1/workspaces", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

// TestListFilterByProject confirms the path :projectId scopes the result.
func TestListFilterByProject(
	t *testing.T,
) {
	reader := &fakeReader{
		list: []domain.Workspace{
			{ID: "w1", ProjectID: "p1", RepoID: "r2"},
			{ID: "w2", ProjectID: "p2", RepoID: "r2"},
		},
	}
	rec := do(newRouter(reader, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p2/repos/r2/workspaces", "")

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "w2", body.Data[0].ID)
}

// TestListFilterByRepo confirms the path :repoId scopes the result.
func TestListFilterByRepo(
	t *testing.T,
) {
	reader := &fakeReader{
		list: []domain.Workspace{
			{ID: "w1", ProjectID: "p1", RepoID: "r1"},
			{ID: "w2", ProjectID: "p1", RepoID: "r2"},
		},
	}
	rec := do(newRouter(reader, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p1/repos/r1/workspaces", "")

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "w1", body.Data[0].ID)
}

func TestListError(
	t *testing.T,
) {
	reader := &fakeReader{listErr: errors.New("db down")}
	rec := do(newRouter(reader, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p1/repos/r1/workspaces", "")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDetailSuccess(
	t *testing.T,
) {
	reader := &fakeReader{get: domain.Workspace{ID: "w9"}}
	rec := do(newRouter(reader, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/w9", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "w9", body.Data.ID)
}

func TestDetailNotFound(
	t *testing.T,
) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	rec := do(newRouter(reader, &fakeHierarchy{}, &fakeRepos{}), http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/nope", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
