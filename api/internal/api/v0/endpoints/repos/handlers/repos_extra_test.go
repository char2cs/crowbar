package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// ---------------------------------------------------------------------------
// repoAvatar / githubSlugFromURL / parseRemoteBranches are unexported pure
// functions. They're exercised indirectly through the exported handlers below
// (buildRepo via Create, githubAvatarURL via fetchAvatar injection, Branches),
// which is how this package's tests reach unexported helpers throughout.
// ---------------------------------------------------------------------------

// runGit runs a git command in dir, failing the test on error. Used to build
// small real git fixture repos so gitDefaultBranch/gitRemoteURL/Branches are
// exercised against actual git plumbing rather than re-implemented mocks.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
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

// TestIcon_ServesFromTheInjectedCrowbarHome pins that icon storage is rooted at
// the home the constructor was given, never at a home the handler resolves
// itself.
func TestIcon_ServesFromTheInjectedCrowbarHome(t *testing.T) {
	devHome := t.TempDir()

	iconPath := filepath.Join(devHome, "projects", "probe-project", "probe-repo", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("dev-home-probe")...)
	require.NoError(t, os.WriteFile(iconPath, png, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "probe-repo", ProjectID: "probe-project", AvatarHasIcon: true}}
	h := repohandlers.New(store, func() (string, error) { return devHome, nil })
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos/:repoId/icon", h.Icon)

	req := httptest.NewRequest(http.MethodGet, "/v0/projects/probe-project/repos/probe-repo/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Icon must read the file under the injected home")
	assert.Equal(t, png, rec.Body.Bytes())
}

// ---------------------------------------------------------------------------
// GitHub avatar resolution: githubSlugFromURL, githubAvatarURL,
// fetchGithubAvatarBytes. These shell out to git and gh, so the tests stub
// both binaries on PATH.
// ---------------------------------------------------------------------------

// writeFakeExe writes an executable shell script named name into dir.
func writeFakeExe(t *testing.T, dir, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-exe PATH stubbing is POSIX-shell only")
	}
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))
}

// withFakePath prepends dir to PATH for the duration of the test.
//
// The git binary is resolved once per process and cached, so the stub only
// takes effect if the cached resolution is dropped alongside the PATH change —
// otherwise the real git runs against a repo path that does not exist and the
// stub is never consulted. gh needs no equivalent: binpath.Resolve is a fresh
// lookup every call.
func withFakePath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	binpath.InvalidateGit()
	t.Cleanup(binpath.InvalidateGit)
}

func TestFetchGithubAvatarBytes_NoOrigin_ReturnsNilNoError(t *testing.T) {
	// A directory that isn't a git repo at all: githubAvatarURL's `git remote
	// get-url origin` fails, so the whole fetch short-circuits before any
	// network call.
	dir := t.TempDir()
	data, ct, err := callFetchGithubAvatarBytes(t, dir)
	assert.Nil(t, data)
	assert.Empty(t, ct)
	assert.NoError(t, err)
}

func TestFetchGithubAvatarBytes_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(append([]byte("\x89PNG\r\n\x1a\n"), []byte("avatar-bytes")...))
	}))
	defer srv.Close()

	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", fmt.Sprintf(`echo %q`, srv.URL+"/avatar.png"))
	withFakePath(t, bin)

	data, ct, err := callFetchGithubAvatarBytes(t, "/any/repo/path")
	require.NoError(t, err)
	assert.Equal(t, "image/png", ct)
	assert.Contains(t, string(data), "avatar-bytes")
}

func TestFetchGithubAvatarBytes_NonOKStatus_ReturnsNilNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", fmt.Sprintf(`echo %q`, srv.URL+"/missing.png"))
	withFakePath(t, bin)

	data, ct, err := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
	assert.NoError(t, err)
}

func TestFetchGithubAvatarBytes_OversizeBody_ReturnsNilNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(make([]byte, (2<<20)+16))
	}))
	defer srv.Close()

	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", fmt.Sprintf(`echo %q`, srv.URL+"/big.png"))
	withFakePath(t, bin)

	data, ct, err := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
	assert.NoError(t, err)
}

func TestFetchGithubAvatarBytes_NetworkFailure_ReturnsNilNoError(t *testing.T) {
	// A URL nothing is listening on: http.DefaultClient.Do returns a transport
	// error, exercising the network-failure branch.
	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", `echo "http://127.0.0.1:1/unreachable.png"`)
	withFakePath(t, bin)

	data, ct, err := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
	assert.NoError(t, err)
}

func TestFetchGithubAvatarBytes_GhAuthFailure_ReturnsNilNoError(t *testing.T) {
	// gh exits non-zero (e.g. not authenticated / not a GitHub remote):
	// githubAvatarURL swallows the error and returns "".
	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", `exit 1`)
	withFakePath(t, bin)

	data, ct, err := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
	assert.NoError(t, err)
}

// callFetchGithubAvatarBytes exercises fetchGithubAvatarBytes (and therefore
// githubAvatarURL/githubSlugFromURL) through PutIconGithub, the only exported
// surface that calls the package-level default via h.fetchAvatar when no
// override is installed.
func callFetchGithubAvatarBytes(t *testing.T, repoPath string) ([]byte, string, error) {
	t.Helper()
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: repoPath}}
	var saveErr error
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return saveErr }

	// New() wires fetchAvatar to the real fetchGithubAvatarBytes; the injected
	// home gives storeIconBytes somewhere to write on the (rare) success path.
	h := repohandlers.New(store, func() (string, error) { return home, nil })
	r := gin.New()
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon/github", h.PutIconGithub)

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		data, err := os.ReadFile(filepath.Join(home, "projects", "p1", "r1", "icon"))
		require.NoError(t, err)
		return data, "image/png", nil
	}
	// 422 == the fetch degraded to (nil, "", nil) or errored.
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	return nil, "", nil
}

// ---------------------------------------------------------------------------
// Branches
// ---------------------------------------------------------------------------

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
	cmd := exec.Command("git", "clone", "-q", upstream, clone)
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
		}},
		nil,
		unusedHome,
	).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequest(http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
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
	h := repohandlers.NewWithDeps(store, nil, nil, nil, unusedHome).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequest(http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
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
	h := repohandlers.NewWithDeps(store, nil, nil, nil, unusedHome).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequest(http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
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
	h := repohandlers.NewWithDeps(store, &fakeBranchProvider{protectedErr: errors.New("gh down")}, nil, nil, unusedHome).
		WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/repos/r1/branches", http.NoBody))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestBranches_StoreLookupError_Returns404(t *testing.T) {
	store := &fakeStore{byKeErr: errors.New("db down")}
	h := repohandlers.NewWithDeps(store, nil, nil, nil, unusedHome).WithRemoteRefresher(enginegit.New())
	r := gin.New()
	r.GET("/v0/repos/:repoId/branches", h.Branches)

	req := httptest.NewRequest(http.MethodGet, "/v0/repos/r1/branches", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// Icon / iconPath extra error paths
// ---------------------------------------------------------------------------

func TestIcon_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	req := httptest.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIcon_HomeResolutionFails_Returns404(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	h := repohandlers.New(store, func() (string, error) { return "", errors.New("no home") })
	r := gin.New()
	r.GET("/v0/projects/:projectId/repos/:repoId/icon", h.Icon)

	req := httptest.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIcon_OversizeOnDiskFile_Returns404(t *testing.T) {
	home := t.TempDir()
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	big := make([]byte, (2<<20)+1)
	require.NoError(t, os.WriteFile(iconPath, big, 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	req := httptest.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIcon_FileMissingOnDisk_Returns404(t *testing.T) {
	home := t.TempDir() // AvatarHasIcon=true but no file written at all
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	r := iconRouter(store, home, nil, http.MethodGet, iconRoute)

	req := httptest.NewRequest(http.MethodGet, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// PutIconEmoji extra error paths
// ---------------------------------------------------------------------------

func TestPutIconEmoji_MultiCharString_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{"emoji":"not-an-emoji"}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIconEmoji_EmptyBody_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIconEmoji_BadJSON_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{not-json`)
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIconEmoji_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }
	r := iconRouter(store, home, nil, http.MethodPut, "/v0/projects/:projectId/repos/:repoId/icon/emoji")

	body := strings.NewReader(`{"emoji":"🦊"}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ---------------------------------------------------------------------------
// DeleteIcon extra error paths
// ---------------------------------------------------------------------------

func TestDeleteIcon_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := iconRouter(store, home, nil, http.MethodDelete, iconRoute)

	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteIcon_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }
	r := iconRouter(store, home, nil, http.MethodDelete, iconRoute)

	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDeleteIcon_HomeResolutionFails_StillClearsFlag(t *testing.T) {
	var saved domain.Repository
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", AvatarHasIcon: true, AvatarEmoji: "🦊"}}
	store.SaveFn = func(_ context.Context, r domain.Repository) error { saved = r; return nil }
	h := repohandlers.New(store, func() (string, error) { return "", errors.New("no home") })
	r := gin.New()
	r.DELETE("/v0/projects/:projectId/repos/:repoId/icon", h.DeleteIcon)

	req := httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, saved.AvatarHasIcon)
	assert.Empty(t, saved.AvatarEmoji)
}

// ---------------------------------------------------------------------------
// PutIcon / readIconFromPath / readIconFromMultipart / storeIconBytes extra
// error paths.
// ---------------------------------------------------------------------------

func TestPutIcon_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := putIconRouter(store, home)

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPutIcon_JSONPath_EmptyPath_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	body, _ := json.Marshal(map[string]string{"path": ""})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_JSONPath_BadJSON_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", strings.NewReader(`{not-json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_Multipart_MissingField_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	var buf bytes.Buffer
	// Valid multipart body but no "icon" part.
	buf.WriteString("--X\r\nContent-Disposition: form-data; name=\"other\"\r\n\r\nvalue\r\n--X--\r\n")
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_Multipart_NonImageContent_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	var buf bytes.Buffer
	buf.WriteString("--X\r\nContent-Disposition: form-data; name=\"icon\"; filename=\"x.png\"\r\nContent-Type: image/png\r\n\r\n")
	buf.WriteString("not actually an image, just text bytes")
	buf.WriteString("\r\n--X--\r\n")
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPutIcon_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }

	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPutIcon_StoreIconBytesFails_HomeResolutionError_Returns500(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	h := repohandlers.New(store, func() (string, error) { return "", errors.New("no home") })
	r := gin.New()
	r.PUT("/v0/projects/:projectId/repos/:repoId/icon", h.PutIcon)

	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPutIcon_LargerThanMax_ViaMultipart_Returns400(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}

	var buf bytes.Buffer
	buf.WriteString("--X\r\nContent-Disposition: form-data; name=\"icon\"; filename=\"big.png\"\r\nContent-Type: image/png\r\n\r\n")
	oversize := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, (2<<20)+16)...)
	buf.Write(oversize)
	buf.WriteString("\r\n--X--\r\n")
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// repoAvatar / githubSlugFromURL via generated-avatar + create-flow paths.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// githubSlugFromURL branch coverage, exercised indirectly through
// githubAvatarURL/PutIconGithub with a fake `git` reporting different remote
// URL shapes.
// ---------------------------------------------------------------------------

func TestGithubAvatarURL_SlugParsing_ViaFakeGit(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
	}{
		{"ssh shorthand git@host:owner/repo.git", "git@github.com:acme/widget.git"},
		{"https URL", "https://github.com/acme/widget.git"},
		{"https URL with no trailing slash-path segment is unrecognised", "https://github.com"},
		{"opaque string with neither git@ nor scheme is unrecognised", "not-a-url-at-all"},
		{"git@ prefix without a colon is unrecognised", "git@github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			writeFakeExe(t, bin, "git", fmt.Sprintf(`echo %q`, tt.remoteURL))
			// gh should never be invoked when the slug can't be parsed; when it
			// can, answer with a benign URL so the fetch degrades cleanly either
			// way (this test only cares about the slug-parse branch, not the
			// download itself).
			writeFakeExe(t, bin, "gh", `echo "http://127.0.0.1:1/unreachable.png"`)
			withFakePath(t, bin)

			data, ct, err := callFetchGithubAvatarBytes(t, "/any/repo/path")
			assert.Nil(t, data)
			assert.Empty(t, ct)
			assert.NoError(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// storeIconBytes: MkdirAll and WriteFile failure branches, exercised through
// PutIcon (the only exported caller of the unexported storeIconBytes).
// ---------------------------------------------------------------------------

func TestPutIcon_MkdirAllFails_Returns500(t *testing.T) {
	home := t.TempDir()
	// Pre-create a *file* (not a directory) at the path storeIconBytes needs to
	// MkdirAll through, so MkdirAll fails with ENOTDIR.
	blocker := filepath.Join(home, "projects", "p1")
	require.NoError(t, os.MkdirAll(filepath.Dir(blocker), 0o755))
	require.NoError(t, os.WriteFile(blocker, []byte("i am a file, not a dir"), 0o644))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPutIcon_WriteFileFails_TargetIsDirectory_Returns500(t *testing.T) {
	home := t.TempDir()
	// Pre-create a *directory* at the exact icon file path, so the parent
	// MkdirAll succeeds but the final os.WriteFile fails (can't write a file
	// where a directory already exists).
	iconPath := filepath.Join(home, "projects", "p1", "r1", "icon")
	require.NoError(t, os.MkdirAll(iconPath, 0o755))

	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	srcPath := filepath.Join(t.TempDir(), "photo.png")
	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("bytes")...)
	require.NoError(t, os.WriteFile(srcPath, png, 0o644))

	body, _ := json.Marshal(map[string]string{"path": srcPath})
	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	putIconRouter(store, home).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ---------------------------------------------------------------------------
// PutIconGithub extra branches not covered by the original suite.
// ---------------------------------------------------------------------------

func TestPutIconGithub_NoLocalPath_Returns422(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: ""}}
	r := iconRouter(store, home, nil, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestPutIconGithub_StoreLookupError_Returns404(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKeErr: errors.New("db down")}
	r := iconRouter(store, home, nil, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPutIconGithub_SaveError_Returns500(t *testing.T) {
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return errors.New("db down") }
	fetch := func(_ context.Context, _ string) ([]byte, string, error) {
		return []byte("AVATARBYTES"), "image/png", nil
	}
	r := iconRouter(store, home, fetch, http.MethodPut,
		"/v0/projects/:projectId/repos/:repoId/icon/github")

	req := httptest.NewRequest(http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// The whole repo — default branch included — is handed to the delete
// lifecycle: the default branch is what tells the cascade which branch to
// re-attach rather than delete (spec §3 P0-1).
func TestRegression_DeleteRepo_HandsTheWholeRepoToTheLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "main"}
	deleter := &fakeRepoDeleter{}
	h := repohandlers.NewWithDeps(&fakeStore{byKey: &repo}, nil, &fakeWSReader{}, nil, unusedHome).
		WithRepoDeleter(deleter)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "projectId", Value: "p1"}, {Key: "repoId", Value: "r1"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1", nil)
	h.DeleteRepo(c)
	h.WaitAsync()

	marked := repo
	marked.Deleting = true
	assert.Equal(t, []domain.Repository{marked}, deleter.deleted)
}

// Unwired, the delete refuses outright rather than accepting work it cannot do.
func TestDeleteRepo_Unwired_Refuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	h := repohandlers.NewWithDeps(store, nil, nil, nil, unusedHome)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "projectId", Value: "p1"}, {Key: "repoId", Value: "r1"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/projects/p1/repos/r1", nil)
	h.DeleteRepo(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
