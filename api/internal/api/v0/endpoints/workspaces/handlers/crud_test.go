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

func TestCreateSuccessFromDefaultBranch(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "new-ws"}}
	repos := &fakeRepos{repo: &domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Path:          "/repo",
		DefaultBranch: "main",
	}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"r1","branch":"feat"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "new-ws", body.Data.ID)
	assert.Equal(t, "r1", hierarchy.gotCreate.RepoID)
	assert.Equal(t, "p1", hierarchy.gotCreate.ProjectID)
	assert.Equal(t, "/repo", hierarchy.gotCreate.RepoPath)
	assert.Equal(t, "feat", hierarchy.gotCreate.Branch)
	assert.Equal(t, "main", hierarchy.gotCreate.ParentBranch)
	assert.Empty(t, hierarchy.gotCreate.ParentID)
}

func TestCreateSuccessFromParent(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{created: domain.Workspace{ID: "child"}}
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main"}}
	reader := &fakeReader{get: domain.Workspace{ID: "parent", Branch: "parent-branch"}}
	rec := do(
		newRouter(reader, hierarchy, repos),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"r1","branch":"feat","parentId":"parent"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "parent", hierarchy.gotCreate.ParentID)
	assert.Equal(t, "parent-branch", hierarchy.gotCreate.ParentBranch)
	assert.Equal(t, "parent", reader.gotID)
}

func TestCreateBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/workspaces",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateMissingRepoID(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/workspaces",
		`{"branch":"feat"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateMissingBranch(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"r1"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateRepoNotFound(
	t *testing.T,
) {
	repos := &fakeRepos{repo: nil}
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"missing","branch":"feat"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCreateRepoLookupError(
	t *testing.T,
) {
	repos := &fakeRepos{err: errors.New("db down")}
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"r1","branch":"feat"}`,
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCreateParentLookupNotFound(
	t *testing.T,
) {
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}}
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, repos),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"r1","branch":"feat","parentId":"nope"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCreateUsecaseError(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{createErr: errors.New("boom")}
	repos := &fakeRepos{repo: &domain.Repository{ID: "r1", Path: "/repo", DefaultBranch: "main"}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, repos),
		http.MethodPost,
		"/v0/workspaces",
		`{"repoId":"r1","branch":"feat"}`,
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDeleteSuccess(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodDelete,
		"/v0/workspaces/w1",
		"",
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "w1", body.Data.ID)
	assert.Equal(t, "w1", hierarchy.gotDeleteID)
}

func TestDeleteError(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{deleteErr: errors.New("boom")}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodDelete,
		"/v0/workspaces/w1",
		"",
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
