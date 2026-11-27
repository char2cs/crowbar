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

	state, err := eng.PollOnView(ctx, "test-ws", dir, "main")
	if err != nil {
		require.Error(t, err)
		return
	}
	assert.IsType(t, provider.ProviderState{}, state)
}

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
