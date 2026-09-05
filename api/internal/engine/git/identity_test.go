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

// installFakeGH puts a fake `gh` script first on PATH so identityFromGH's
// `exec.CommandContext(ctx, binpath.Resolve("gh"), ...)` finds it via the plain
// PATH lookup — binpath.Resolve tries exec.LookPath before ever consulting its
// hardcoded well-known dirs (/opt/homebrew/bin, /usr/local/bin, /usr/bin), so a
// hit on PATH is never shadowed by whatever real `gh` happens to be installed
// on the machine running the test. That is what makes these tests deterministic
// regardless of whether the host has `gh` installed or authenticated.
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

// gitConfigOnlyEnv points HOME and every git config search path at an empty
// directory so `git config user.name` cannot pick up the real developer's
// global config (which is set on most machines this test could run on) —
// without this, a "no user.name configured" test would only pass by accident
// of whoever's machine or CI image runs it.
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
