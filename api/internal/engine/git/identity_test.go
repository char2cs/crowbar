package git

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func installFakeGH(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary PATH shadowing is POSIX only")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))
	t.Setenv("PATH", dir)
}

func TestIdentityFromGH_SuccessTrimsFields(t *testing.T) {
	installFakeGH(t, `echo '{"login":"  octocat  ","name":"  The Octocat  ","avatar_url":"  https://example.com/a.png  "}'`)

	id, ok := identityFromGH(context.Background(), t.TempDir())

	require.True(t, ok)
	assert.Equal(t, "octocat", id.Login)
	assert.Equal(t, "The Octocat", id.DisplayName)
	assert.Equal(t, "https://example.com/a.png", id.AvatarURL)
}

func TestIdentityFromGH_NonZeroExitFallsThrough(t *testing.T) {
	installFakeGH(t, `exit 1`)

	id, ok := identityFromGH(context.Background(), t.TempDir())

	assert.False(t, ok)
	assert.Zero(t, id)
}

func TestIdentityFromGH_MalformedJSONFallsThrough(t *testing.T) {
	installFakeGH(t, `echo 'not json'`)

	id, ok := identityFromGH(context.Background(), t.TempDir())

	assert.False(t, ok)
	assert.Zero(t, id)
}

func gitConfigOnlyEnv(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(empty, "no-such-gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(empty, "no-such-systemconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func TestIdentityFromGitConfig_ReadsLocalUserName(t *testing.T) {
	gitConfigOnlyEnv(t)
	dir := t.TempDir()
	require.NoError(t, gitexec.RequireSuccess("git init", gitexec.Git(context.Background(), dir, "init")))
	require.NoError(t, gitexec.RequireSuccess("git config", gitexec.Git(context.Background(), dir, "config", "user.name", "  Test User  ")))

	id := identityFromGitConfig(context.Background(), dir)

	assert.Equal(t, "Test User", id.DisplayName)
	assert.Empty(t, id.Login)
	assert.Empty(t, id.AvatarURL)
}

func TestIdentityFromGitConfig_NoConfigReturnsEmptyIdentity(t *testing.T) {
	gitConfigOnlyEnv(t)
	dir := t.TempDir()

	id := identityFromGitConfig(context.Background(), dir)

	assert.Zero(t, id)
}

func TestCurrentIdentity_FallsBackToGitConfigWhenGHFails(t *testing.T) {
	installFakeGH(t, `exit 1`)
	gitConfigOnlyEnv(t)
	dir := t.TempDir()
	require.NoError(t, gitexec.RequireSuccess("git init", gitexec.Git(context.Background(), dir, "init")))
	require.NoError(t, gitexec.RequireSuccess("git config", gitexec.Git(context.Background(), dir, "config", "user.name", "Fallback User")))

	id := CurrentIdentity(context.Background(), dir)

	assert.Equal(t, "Fallback User", id.DisplayName)
	assert.Empty(t, id.Login)
}

func TestCurrentIdentity_PrefersGH(t *testing.T) {
	installFakeGH(t, `echo '{"login":"octocat","name":"The Octocat","avatar_url":"https://example.com/a.png"}'`)
	dir := t.TempDir()

	id := CurrentIdentity(context.Background(), dir)

	assert.Equal(t, "octocat", id.Login)
	assert.Equal(t, "The Octocat", id.DisplayName)
}
