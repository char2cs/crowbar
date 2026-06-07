package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestProtectedBranches_200(t *testing.T) {
	prov := &fakeProvider{branches: []string{"main", "release"}}
	r := newRouter(prov, okWSReader())

	rec := do(t, r, "/v0/repos/r1/protected-branches")
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	var branches []string
	require.NoError(t, json.Unmarshal(env.Data, &branches))
	assert.Equal(t, []string{"main", "release"}, branches)
}

func TestProtectedBranches_PassesRepoRootPath(t *testing.T) {
	prov := &fakeProvider{branches: []string{"main"}}
	repos := &fakeRepoReader{repo: &domain.Repository{ID: "r1", Path: "/repo-root"}}
	r := newRouterWithRepos(prov, okWSReader(), repos)

	rec := do(t, r, "/v0/repos/r1/protected-branches")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/repo-root", prov.lastRepoPath)
}

func TestProtectedBranches_EmptyArrayNotNull(t *testing.T) {
	r := newRouter(&fakeProvider{}, okWSReader())

	rec := do(t, r, "/v0/repos/r1/protected-branches")
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	assert.Equal(t, "[]", string(env.Data))
}

func TestProtectedBranches_RepoNotFound_404(t *testing.T) {
	repos := &fakeRepoReader{repo: nil}
	r := newRouterWithRepos(&fakeProvider{}, okWSReader(), repos)

	rec := do(t, r, "/v0/repos/ghost/protected-branches")
	require.Equal(t, http.StatusNotFound, rec.Code)

	env := decode(t, rec)
	assert.False(t, env.Success)
	assert.Equal(t, "repo not found", env.Error)
}

func TestProtectedBranches_RepoStoreError_500(t *testing.T) {
	repos := &fakeRepoReader{err: errBoom}
	r := newRouterWithRepos(&fakeProvider{}, okWSReader(), repos)

	rec := do(t, r, "/v0/repos/r1/protected-branches")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, decode(t, rec).Success)
}

func TestProtectedBranches_EngineError_500(t *testing.T) {
	r := newRouter(&fakeProvider{branchErr: errBoom}, okWSReader())

	rec := do(t, r, "/v0/repos/r1/protected-branches")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, decode(t, rec).Success)
}

func TestProtectedBranches_NilEngine_503(t *testing.T) {
	r := newRouter(nil, okWSReader())

	rec := do(t, r, "/v0/repos/r1/protected-branches")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	env := decode(t, rec)
	assert.False(t, env.Success)
	assert.Equal(t, "provider engine not available", env.Error)
}
