package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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

type envelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

func probe[T any](t *testing.T, reqCtx context.Context, tool string) envelope[T] {
	t.Helper()
	r := gin.New()
	h := systemhandlers.NewHandler(context.Background())
	r.GET("/v0/system/prerequisites/:tool", h.Prerequisite)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(reqCtx, http.MethodGet, "/v0/system/prerequisites/"+tool, http.NoBody))
	require.Equal(t, http.StatusOK, rec.Code, "a missing toolchain is data, not an error")
	var got envelope[T]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

func TestPrerequisites_UnavailableToolsReportNotInstalled(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	git := probe[systemhandlers.GitStatus](t, ctx, "git")
	assert.True(t, git.Success)
	assert.False(t, git.Data.Installed)
	assert.Empty(t, git.Data.Version)
	for _, cli := range []string{"gh", "glab"} {
		got := probe[systemhandlers.CLIStatus](t, ctx, cli)
		assert.False(t, got.Data.Installed, cli)
		assert.False(t, got.Data.Authed, cli)
	}
}

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

func TestPrerequisites_ReportsGitVersionWhenInstalled(
	t *testing.T,
) {
	got := probe[systemhandlers.GitStatus](t, context.Background(), "git")
	assert.True(t, got.Data.Installed, "the real git on PATH must be detected as installed")
	assert.NotEmpty(t, got.Data.Version, "a successful `git --version` must report a version string")
}

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

	gh := probe[systemhandlers.CLIStatus](t, context.Background(), "gh")
	assert.True(t, gh.Data.Installed)
	assert.True(t, gh.Data.Authed, "gh's auth status call succeeded, so Authed must be true")
	glab := probe[systemhandlers.CLIStatus](t, context.Background(), "glab")
	assert.True(t, glab.Data.Installed)
	assert.False(t, glab.Data.Authed, "glab never succeeded at auth status, so it is installed but not Authed")
}

func TestPrerequisites_AHungProbeIsBoundedAndReadsAsNotAuthed(t *testing.T) {
	binDir := t.TempDir()
	writeFakeCLI(t, binDir, "gh", `
if [ "$1" = "--version" ]; then
  exit 0
fi
sleep 30
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := gin.New()
	h := systemhandlers.NewHandlerWithProbeTimeout(200 * time.Millisecond)
	r.GET("/v0/system/prerequisites/:tool", h.Prerequisite)

	start := time.Now()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/system/prerequisites/gh", http.NoBody))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Less(t, time.Since(start), 5*time.Second, "the hung probe must not hold the answer")
	var got struct {
		Data systemhandlers.CLIStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Data.Installed)
	assert.False(t, got.Data.Authed)
}

func TestPrerequisite_AnswersOneToolAndRefusesAnUnknownOne(t *testing.T) {
	r := gin.New()
	h := systemhandlers.NewHandler(context.Background())
	r.GET("/v0/system/prerequisites/:tool", h.Prerequisite)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/system/prerequisites/git", http.NoBody))
	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Data systemhandlers.GitStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Data.Installed)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/system/prerequisites/svn", http.NoBody))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
