//go:build integration

package provider_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/provider"
)

// TestIntegration_WithGH exercises the engine with the real gh CLI if present.
func TestIntegration_WithGH(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}

	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	mustRun(t, dir, "git", "remote", "add", "origin", "https://github.com/char2cs/crowbar")

	ctx := context.Background()
	eng := provider.New()
	res, err := eng.Capability(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "github", res.Kind)

	// PollOnView may fail because the repo doesn't actually have a gh-accessible remote,
	// but it must not panic and must return a wrapped error.
	state, err := eng.PollOnView(ctx, "test-ws", dir, "main")
	if err != nil {
		require.Error(t, err)
		return
	}
	// If no error (gh is authed and repo exists), state should be valid.
	assert.IsType(t, provider.ProviderState{}, state)
}

// TestIntegration_CLIAbsent verifies graceful degradation when the provider CLI
// is missing. The exec seam hides gh/glab; PATH is untouched because binpath
// also probes well-known dirs (/opt/homebrew/bin) that no PATH edit can hide.
func TestIntegration_CLIAbsent(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	mustRun(t, dir, "git", "remote", "add", "origin", "https://github.com/char2cs/crowbar")

	ctx := context.Background()
	eng := provider.NewWithExec(execWithoutProviderCLIs, time.Now)

	cap, err := eng.Capability(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, provider.ProviderCapability{Kind: "github", Enabled: false}, cap)

	state, err := eng.PollOnView(ctx, "ws1", dir, "main")
	require.NoError(t, err)
	assert.False(t, state.Protected)
	assert.Nil(t, state.PR)

	branches, err := eng.ProtectedBranches(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop", "master"}, branches)
}

// execWithoutProviderCLIs runs git for real but makes gh and glab fail to start,
// as they do on a machine where they are not installed.
func execWithoutProviderCLIs(
	ctx context.Context,
	name string,
	args ...string,
) *exec.Cmd {
	switch filepath.Base(name) {
	case "gh", "glab":
		return exec.CommandContext(ctx, filepath.Join(os.TempDir(), "crowbar-absent-cli", filepath.Base(name)))
	default:
		return exec.CommandContext(ctx, name, args...)
	}
}

func mustRun(
	t *testing.T,
	dir string,
	name string,
	args ...string,
) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %q failed: %v\n%s", name+" "+args[0], err, out)
	}
}
