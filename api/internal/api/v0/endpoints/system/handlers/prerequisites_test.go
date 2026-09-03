package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// envelope mirrors the {success, data} query envelope every v0 read answers with.
type envelope struct {
	Success bool                             `json:"success"`
	Data    systemhandlers.PrerequisitesData `json:"data"`
}

// doPrerequisites serves GET /v0/system/prerequisites with reqCtx as the request
// context, so a test can decide whether the probes are allowed to run at all.
func doPrerequisites(
	reqCtx context.Context,
) *httptest.ResponseRecorder {
	r := gin.New()
	h := systemhandlers.NewHandler(context.Background())
	r.GET("/v0/system/prerequisites", h.Prerequisites)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/system/prerequisites", http.NoBody)
	r.ServeHTTP(rec, req.WithContext(reqCtx))
	return rec
}

// TestPrerequisites_UnavailableToolsReportNotInstalled pins the degraded answer:
// when no probe can run, the endpoint still answers 200 with every tool reported
// absent rather than erroring. A CANCELLED request context is what makes this
// deterministic — every exec.CommandContext fails immediately, on any machine,
// with no dependency on which CLIs happen to be installed and no network call
// (a real `gh auth status` would make one).
func TestPrerequisites_UnavailableToolsReportNotInstalled(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := doPrerequisites(ctx)

	require.Equal(t, http.StatusOK, rec.Code, "a missing toolchain is data, not an error")
	var got envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Success)
	assert.False(t, got.Data.Git.Installed)
	assert.Empty(t, got.Data.Git.Version)
	assert.False(t, got.Data.Gh.Installed)
	assert.False(t, got.Data.Gh.Authed)
	assert.False(t, got.Data.Glab.Installed)
	assert.False(t, got.Data.Glab.Authed)
}

// writeFakeCLI drops an executable shell script at dir/name, standing in for a
// provider CLI (gh/glab) that may not be installed, authenticated, or
// otherwise controllable from a test in any other way.
func writeFakeCLI(
	t *testing.T,
	dir string,
	name string,
	script string,
) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))
}

// TestPrerequisites_ReportsGitVersionWhenInstalled exercises checkGit's
// success path: a real, uncancelled context lets the real git on this
// machine's PATH run for real (every dev box and CI runner has git — this IS a
// git repo), so the response must report it installed with a non-empty
// version string, not just the degraded "absent" shape the cancelled-context
// test above pins.
func TestPrerequisites_ReportsGitVersionWhenInstalled(
	t *testing.T,
) {
	rec := doPrerequisites(context.Background())

	require.Equal(t, http.StatusOK, rec.Code)
	var got envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Data.Git.Installed, "the real git on PATH must be detected as installed")
	assert.NotEmpty(t, got.Data.Git.Version, "a successful `git --version` must report a version string")
}

// TestPrerequisites_DetectsInstalledAndUnauthenticatedCLI exercises checkCLI's
// success path for BOTH of its sub-cases in one pass: a "gh" that answers
// --version successfully is Installed, and whether its "auth status" call also
// succeeds decides Authed independently — "glab" here is installed but never
// told to succeed authentication, so it must come back Installed but not
// Authed, distinguishing "not on the machine at all" from "on the machine but
// not logged in".
func TestPrerequisites_DetectsInstalledAndUnauthenticatedCLI(
	t *testing.T,
) {
	binDir := t.TempDir()
	writeFakeCLI(t, binDir, "gh", `
if [ "$1" = "--version" ]; then
  exit 0
fi
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  exit 0
fi
exit 1
`)
	writeFakeCLI(t, binDir, "glab", `
if [ "$1" = "--version" ]; then
  exit 0
fi
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	rec := doPrerequisites(context.Background())

	require.Equal(t, http.StatusOK, rec.Code)
	var got envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Data.Gh.Installed)
	assert.True(t, got.Data.Gh.Authed, "gh's auth status call succeeded, so Authed must be true")
	assert.True(t, got.Data.Glab.Installed)
	assert.False(t, got.Data.Glab.Authed, "glab's auth status was never asked to succeed, so Authed must be false")
}
