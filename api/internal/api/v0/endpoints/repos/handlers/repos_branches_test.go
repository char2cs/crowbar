package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// runGit runs a git command in dir, failing the test on error. Used to build
// small real git fixture repos so gitDefaultBranch/gitRemoteURL/Branches are
// exercised against actual git plumbing rather than re-implemented mocks.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

// initRepo creates a minimal git repo at dir on branch "main" with one commit.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644))
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-q", "-m", "init")
}

// initRemoteTrackingRepo builds an upstream repo with a main + feature branch
// and clones it, so the clone has real refs/remotes/origin/* entries for
// `git branch -r` to list.
func initRemoteTrackingRepo(t *testing.T) (clonePath string) {
	t.Helper()
	base := t.TempDir()
	upstream := filepath.Join(base, "upstream")
	initRepo(t, upstream)
	runGit(t, upstream, "checkout", "-q", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "b.txt"), []byte("hi2"), 0o644))
	runGit(t, upstream, "add", "b.txt")
	runGit(t, upstream, "commit", "-q", "-m", "feature work")
	runGit(t, upstream, "checkout", "-q", "main")

	clone := filepath.Join(base, "clone")
	cmd := exec.CommandContext(t.Context(), "git", "clone", "-q", upstream, clone)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git clone: %s", out)
	return clone
}

func TestBranches_Success_AnnotatesProtectionAndWorkspace(t *testing.T) {
	clone := initRemoteTrackingRepo(t)
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: clone}}
	h := repohandlers.NewWithDeps(
		store,
		&fakeBranchProvider{protected: []string{"main"}},
		&fakeWSReader{workspaces: []domain.Workspace{
			{RepoID: "r1", Branch: "feature", IsDefault: false},
			// A default workspace on "main" must NOT count as hasWorkspace.
			{RepoID: "r1", Branch: "main", IsDefault: true},
			// A different repo's workspace must not leak in.
			{RepoID: "other", Branch: "feature", IsDefault: false},
		}}, nil,
		nil,
		unusedHome,
	).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data []repohandlers.BranchEntry `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	byName := map[string]repohandlers.BranchEntry{}
	for _, e := range body.Data {
		byName[e.Name] = e
	}
	require.Contains(t, byName, "main")
	require.Contains(t, byName, "feature")
	assert.True(t, byName["main"].IsProtected)
	assert.False(t, byName["main"].HasWorkspace, "default workspace must not count as hasWorkspace")
	assert.False(t, byName["feature"].IsProtected)
	assert.True(t, byName["feature"].HasWorkspace)
}

func TestBranches_NoProviderOrWorkspaceReader_StillListsBranches(t *testing.T) {
	clone := initRemoteTrackingRepo(t)
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: clone}}
	// NewWithDeps with nil provider/wsReader exercises both "if h.provider !=
	// nil" / "if h.wsReader != nil" false branches.
	h := repohandlers.NewWithDeps(store, nil, nil, nil, nil, unusedHome).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []repohandlers.BranchEntry `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	for _, e := range body.Data {
		assert.False(t, e.IsProtected)
		assert.False(t, e.HasWorkspace)
	}
	assert.NotEmpty(t, body.Data)
}

func TestBranches_GitCommandFails_Returns500(t *testing.T) {
	// repo.Path points at a directory that is not a git repository: `git
	// branch -r` fails.
	dir := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: dir}}
	h := repohandlers.NewWithDeps(store, nil, nil, nil, nil, unusedHome).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A protection lookup that fails used to be read as "nothing is protected",
// so the picker offered a protected branch as an ordinary import. It is an
// error the caller sees.
func TestRegression_Branches_ProtectionLookupFailureIsNotUnprotected(t *testing.T) {
	clone := initRemoteTrackingRepo(t)
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: clone}}
	h := repohandlers.NewWithDeps(store, &fakeBranchProvider{protectedErr: errors.New("gh down")}, nil, nil, nil, unusedHome).
		WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos/r1/branches", http.NoBody))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestBranches_StoreLookupError_Returns404(t *testing.T) {
	store := &fakeStore{byKeErr: errors.New("db down")}
	h := repohandlers.NewWithDeps(store, nil, nil, nil, nil, unusedHome).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
