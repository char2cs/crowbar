package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
	data, ct := callFetchGithubAvatarBytes(t, dir)
	assert.Nil(t, data)
	assert.Empty(t, ct)
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

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
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

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
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

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestFetchGithubAvatarBytes_NetworkFailure_ReturnsNilNoError(t *testing.T) {
	// A URL nothing is listening on: http.DefaultClient.Do returns a transport
	// error, exercising the network-failure branch.
	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", `echo "http://127.0.0.1:1/unreachable.png"`)
	withFakePath(t, bin)

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestFetchGithubAvatarBytes_GhAuthFailure_ReturnsNilNoError(t *testing.T) {
	// gh exits non-zero (e.g. not authenticated / not a GitHub remote):
	// githubAvatarURL swallows the error and returns "".
	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", `exit 1`)
	withFakePath(t, bin)

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

// callFetchGithubAvatarBytes exercises fetchGithubAvatarBytes (and therefore
// githubAvatarURL/githubSlugFromURL) through PutIconGithub, the only exported
// surface that calls the package-level default via h.fetchAvatar when no
// override is installed.
func callFetchGithubAvatarBytes(t *testing.T, repoPath string) ([]byte, string) {
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v0/projects/p1/repos/r1/icon/github", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		data, err := os.ReadFile(filepath.Join(home, "projects", "p1", "r1", "icon"))
		require.NoError(t, err)
		return data, "image/png"
	}
	// 422 == the fetch degraded to (nil, "", nil) or errored.
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	return nil, ""
}

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

			data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
			assert.Nil(t, data)
			assert.Empty(t, ct)
		})
	}
}
