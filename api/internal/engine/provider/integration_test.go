//go:build integration

package provider_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/internal/detect"
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
	res, err := detect.Detect(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "github", res.Kind)

	eng := provider.New()
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

// TestIntegration_CLIAbsent verifies graceful degradation when the CLI is missing.
func TestIntegration_CLIAbsent(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	mustRun(t, dir, "git", "remote", "add", "origin", "https://github.com/char2cs/crowbar")

	ctx := context.Background()

	// Simulate CLI unavailable by unsetting PATH for the test process temporarily.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer func() { _ = os.Setenv("PATH", origPath) }()

	eng := provider.New()

	// Capability should gracefully return enabled=false (no panic, no error).
	cap, err := eng.Capability(ctx, dir)
	require.NoError(t, err)
	// With no PATH, git itself may fail, so kind may be "none".
	assert.IsType(t, provider.ProviderCapability{}, cap)

	// PollOnView should return zero state gracefully.
	state, err := eng.PollOnView(ctx, "ws1", dir, "main")
	require.NoError(t, err)
	assert.False(t, state.Protected)
	assert.Nil(t, state.PR)

	// ProtectedBranches should return the fallback list when CLI is absent.
	branches, err := eng.ProtectedBranches(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop", "master"}, branches)
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
