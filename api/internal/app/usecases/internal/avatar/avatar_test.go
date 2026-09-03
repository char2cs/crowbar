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

// TestRegression_DownloadBytes_RejectsOversize proves pass-8: an avatar host that
// streams more than the cap is rejected (soft failure) rather than read into
// memory, so a malicious/misconfigured URL can't OOM the import path.
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

// TestDownloadBytes_AcceptsUnderCap proves a normal (small) avatar still downloads.
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
	assert.Len(t, Palette(), paletteSize)
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
	assert.Contains(t, Palette(), a)
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
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got) // svg wins over ico
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
	// A bare temp dir is not a git repo, so `git remote get-url origin` fails
	// and the helper must degrade to (nil, "", nil) — no error swallowed
	// wrongly, no bytes fabricated.
	dir := t.TempDir()
	data, ct, err := FetchOwnerAvatarBytes(context.Background(), dir)
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

// TestOwnerAvatarURL_UnparsableRemoteDegradesEmpty covers the branch where `git
// remote get-url origin` DOES succeed (a real git repo with a real origin
// configured — no network call, this is a local config read) but the URL is not
// a shape githubSlug can parse. The helper must degrade to "" rather than error,
// since a soft failure here falls back to a generated avatar (see
// FetchOwnerAvatarBytes's doc comment).
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

// writeFakeGh drops an executable shell script named "gh" at dir, standing in
// for the real CLI so its exit code/output are under the test's control —
// there is no exported seam on ownerAvatarURL to inject a fake exec.Cmd, so
// this is the only way to drive its `gh api ... --jq .owner.avatar_url` call.
func writeFakeGh(t *testing.T, dir string, script string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n"+script+"\n"), 0o755))
}

// TestOwnerAvatarURL_GhFailureDegradesEmpty covers the branch where the origin
// remote DOES parse into a slug but the `gh api` call itself fails (not
// installed, not authenticated, rate-limited, …) — the helper must degrade to
// "" rather than error, exactly like the earlier "unparsable remote" case.
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

// TestFetchOwnerAvatarBytes_DownloadsViaResolvedURL is the full happy path
// through both ownerAvatarURL (git remote parses, gh resolves an avatar URL)
// and downloadBytes (the resolved URL is fetched for real) — the only test in
// this file that reaches FetchOwnerAvatarBytes's actual download call rather
// than its early "no URL" degrade.
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

// TestGithubSlug_SSHRemote covers the git@ SSH remote shape, including the
// success path that returns "owner/repo" from the colon-separated form.
func TestGithubSlug_SSHRemote(t *testing.T) {
	slug, err := githubSlug("git@github.com:owner/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", slug)
}

// TestGithubSlug_HTTPSRemote covers the "://" HTTPS remote shape.
func TestGithubSlug_HTTPSRemote(t *testing.T) {
	slug, err := githubSlug("https://github.com/owner/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", slug)
}

// TestGithubSlug_GitPrefixWithoutColon covers a "git@" prefix that does not
// split into exactly two SplitN parts (no ':' at all) — it must fall through to
// the "://" check and then to the final unrecognised-URL error, not panic on a
// short slice.
func TestGithubSlug_GitPrefixWithoutColon(t *testing.T) {
	_, err := githubSlug("git@hostwithnocolon")
	require.Error(t, err)
}

// TestGithubSlug_SchemeWithNoPathSeparator covers a "://" URL with no slash
// after the host, which must fall through rather than return a malformed slug.
func TestGithubSlug_SchemeWithNoPathSeparator(t *testing.T) {
	_, err := githubSlug("https://onlyahost")
	require.Error(t, err)
}

// TestGithubSlug_Unrecognised covers the final catch-all error for a remote
// that matches neither the SSH nor HTTPS shape.
func TestGithubSlug_Unrecognised(t *testing.T) {
	_, err := githubSlug("not a remote url at all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognised remote URL")
}

// TestDownloadBytes_RequestConstructionError covers http.NewRequestWithContext
// failing: a URL containing a raw control character is rejected by net/url
// before any network I/O happens, so downloadBytes must degrade to (nil, "",
// nil) rather than propagate the error (this is a best-effort helper).
func TestDownloadBytes_RequestConstructionError(t *testing.T) {
	data, ct, err := downloadBytes(context.Background(), "http://example.com/\n")
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

// TestDownloadBytes_TransportError covers http.DefaultClient.Do failing (here,
// a connection refused against a port nothing listens on) — also a soft
// failure, not a propagated error.
func TestDownloadBytes_TransportError(t *testing.T) {
	data, ct, err := downloadBytes(context.Background(), "http://127.0.0.1:1/")
	require.NoError(t, err)
	assert.Nil(t, data)
	assert.Empty(t, ct)
}

// TestDownloadBytes_NonOKStatus covers a reachable server that answers with a
// non-200 status (here, 404) — the body must be discarded, not returned as if
// it were a valid avatar.
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

// TestDownloadBytes_ReadError covers io.ReadAll failing mid-body: the server
// declares a Content-Length far larger than what it actually sends, so the
// connection closes before the client has read the promised bytes.
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

// TestDownloadBytes_DefaultsContentTypeWhenAbsent covers the branch where the
// upstream response carries no Content-Type at all (a 200 with an empty body:
// Go's server only auto-sniffs a Content-Type on an actual Write call, so a
// bodiless response leaves the header unset). downloadBytes must default it to
// image/png rather than return an empty content-type string.
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
