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

func writeFakeExe(t *testing.T, dir, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-exe PATH stubbing is POSIX-shell only")
	}
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))
}

func withFakePath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	binpath.InvalidateGit()
	t.Cleanup(binpath.InvalidateGit)
}

func TestFetchGithubAvatarBytes_NoOrigin_ReturnsNilNoError(t *testing.T) {
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
	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", `echo "http://127.0.0.1:1/unreachable.png"`)
	withFakePath(t, bin)

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestFetchGithubAvatarBytes_GhAuthFailure_ReturnsNilNoError(t *testing.T) {
	bin := t.TempDir()
	writeFakeExe(t, bin, "git", `echo "https://github.com/acme/widget.git"`)
	writeFakeExe(t, bin, "gh", `exit 1`)
	withFakePath(t, bin)

	data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func callFetchGithubAvatarBytes(t *testing.T, repoPath string) ([]byte, string) {
	t.Helper()
	home := t.TempDir()
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: repoPath}}
	var saveErr error
	store.SaveFn = func(_ context.Context, _ domain.Repository) error { return saveErr }

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
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	return nil, ""
}

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
			writeFakeExe(t, bin, "gh", `echo "http://127.0.0.1:1/unreachable.png"`)
			withFakePath(t, bin)

			data, ct := callFetchGithubAvatarBytes(t, "/any/repo/path")
			assert.Nil(t, data)
			assert.Empty(t, ct)
		})
	}
}
