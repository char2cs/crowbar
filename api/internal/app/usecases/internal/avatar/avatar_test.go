package avatar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_DownloadBytes_RejectsOversize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(make([]byte, maxAvatarBytes+1024))
	}))
	defer srv.Close()

	data, ct, err := downloadBytes(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Nil(t, data, "an oversize avatar body must be rejected, not returned")
	assert.Empty(t, ct)
}

func TestDownloadBytes_AcceptsUnderCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("small-png-bytes"))
	}))
	defer srv.Close()

	data, ct, err := downloadBytes(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "small-png-bytes", string(data))
	assert.Equal(t, "image/png", ct)
}

func TestPaletteSizeMatchesConst(t *testing.T) {
	assert.Len(t, palette(), paletteSize)
}

func TestLabel_FirstAlnumUppercased(t *testing.T) {
	assert.Equal(t, "C", Label("crowbar"))
	assert.Equal(t, "9", Label("9front"))
	assert.Equal(t, "A", Label("  api"))
	assert.Equal(t, "?", Label(""))
	assert.Equal(t, "?", Label("---"))
}

func TestColor_StableForSameName(t *testing.T) {
	a := Color("crowbar")
	b := Color("crowbar")
	assert.Equal(t, a, b)
	assert.Contains(t, palette(), a)
}

func TestColor_DistributesAcrossPalette(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		seen[Color(n)] = true
	}
	assert.GreaterOrEqual(t, len(seen), 2)
}

func TestScanRepoIcon_FindsFaviconSVG(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644))
	got := ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got)
}

func TestScanRepoIcon_PriorityOrder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.ico"), []byte("ico"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644))
	got := ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got)
}

func TestScanRepoIcon_PublicSubdir(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "public")
	require.NoError(t, os.Mkdir(pub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pub, "logo.png"), []byte("png"), 0o644))
	got := ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(pub, "logo.png"), got)
}

func TestScanRepoIcon_NoMatch_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := ScanRepoIcon(dir)
	assert.Empty(t, got)
}

func TestFetchOwnerAvatarBytes_DegradesEmptyWhenNoGh(t *testing.T) {
	dir := t.TempDir()
	data, ct, err := FetchOwnerAvatarBytes(context.Background(), dir)
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestOwnerAvatarURL_UnparsableRemoteDegradesEmpty(t *testing.T) {
	dir := t.TempDir()
	runGitNoErr(t, dir, "init")
	runGitNoErr(t, dir, "remote", "add", "origin", "not-a-recognisable-remote")

	got := ownerAvatarURL(context.Background(), dir)
	assert.Empty(t, got, "an origin githubSlug cannot parse must degrade to \"\", not panic")
}

func runGitNoErr(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func writeFakeGh(t *testing.T, dir, script string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n"+script+"\n"), 0o755))
}

func TestOwnerAvatarURL_GhFailureDegradesEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a unix shell script standing in for gh")
	}
	dir := t.TempDir()
	runGitNoErr(t, dir, "init")
	runGitNoErr(t, dir, "remote", "add", "origin", "git@github.com:owner/repo.git")

	binDir := t.TempDir()
	writeFakeGh(t, binDir, "exit 1")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got := ownerAvatarURL(context.Background(), dir)
	assert.Empty(t, got, "a failing gh call must degrade to \"\", not error")
}

func TestFetchOwnerAvatarBytes_DownloadsViaResolvedURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a unix shell script standing in for gh")
	}
	avatarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("owner-avatar-bytes"))
	}))
	defer avatarSrv.Close()

	dir := t.TempDir()
	runGitNoErr(t, dir, "init")
	runGitNoErr(t, dir, "remote", "add", "origin", "git@github.com:owner/repo.git")

	binDir := t.TempDir()
	writeFakeGh(t, binDir, "echo '"+avatarSrv.URL+"'")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	data, ct, err := FetchOwnerAvatarBytes(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, "owner-avatar-bytes", string(data))
	assert.Equal(t, "image/png", ct)
}

func TestGithubSlug_SSHRemote(t *testing.T) {
	slug, err := githubSlug("git@github.com:owner/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", slug)
}

func TestGithubSlug_HTTPSRemote(t *testing.T) {
	slug, err := githubSlug("https://github.com/owner/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", slug)
}

func TestGithubSlug_GitPrefixWithoutColon(t *testing.T) {
	_, err := githubSlug("git@hostwithnocolon")
	require.Error(t, err)
}

func TestGithubSlug_SchemeWithNoPathSeparator(t *testing.T) {
	_, err := githubSlug("https://onlyahost")
	require.Error(t, err)
}

func TestGithubSlug_Unrecognised(t *testing.T) {
	_, err := githubSlug("not a remote url at all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognised remote URL")
}

func TestDownloadBytes_RequestConstructionError(t *testing.T) {
	data, ct, err := downloadBytes(context.Background(), "http://example.com/\n")
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestDownloadBytes_TransportError(t *testing.T) {
	data, ct, err := downloadBytes(context.Background(), "http://127.0.0.1:1/")
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestDownloadBytes_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	data, ct, err := downloadBytes(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestDownloadBytes_ReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()

	data, ct, err := downloadBytes(context.Background(), srv.URL)
	require.NoError(t, err, "a truncated body is a soft failure, not a propagated error")
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

func TestDownloadBytes_DefaultsContentTypeWhenAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on unix-style bodiless response semantics")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	data, ct, err := downloadBytes(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Empty(t, data)
	assert.Equal(t, "image/png", ct, "an absent Content-Type must default to image/png")
}
