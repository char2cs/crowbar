package modeldiscovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// sentinelDescriptor's command, if actually run, writes sentinel to disk —
// the only proof a test needs that a fork happened at all, rather than
// merely trusting the returned error.
func sentinelDescriptor(sentinel string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "guarded"}
	d.Spawn.Cmd = "sh"
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		Command: []string{"-c", "touch " + sentinel + " && echo {}"},
		Adapter: spec.ModelDiscoverAdapterJSON,
	}}
	return d
}

func TestGuardedProbe_DoesNotForkUnderGoTest(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "ran")

	_, err := guardedProbe(context.Background(), sentinelDescriptor(sentinel), models.ProbeOptions{Cwd: dir}, nil)

	require.ErrorIs(t, err, errProbingDisabledUnderTest)
	_, statErr := os.Stat(sentinel)
	assert.True(t, os.IsNotExist(statErr), "guardedProbe must never fork the descriptor's own command under go test")
}

// TestGuardedProbe_LiveEnvVarOptsBackIntoARealFork proves the guard is a real
// toggle, not just a hardcoded refusal: the same descriptor that testing.Testing()
// alone blocks above actually runs once the escape hatch is set.
func TestGuardedProbe_LiveEnvVarOptsBackIntoARealFork(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "ran")
	t.Setenv(liveProbeEnvVar, "1")

	_, err := guardedProbe(context.Background(), sentinelDescriptor(sentinel), models.ProbeOptions{Cwd: dir}, nil)

	require.NoError(t, err)
	_, statErr := os.Stat(sentinel)
	assert.NoError(t, statErr, "the escape hatch must actually run the real probe")
}

// TestNewCache_DefaultProbeIsGuarded pins that NewCache's own default probe
// field is the test-safe wrapper, not the raw Probe — the actual production
// wiring this whole guard depends on; every cache_test.go test overrides
// c.probe, so nothing else would catch a regression here.
func TestNewCache_DefaultProbeIsGuarded(t *testing.T) {
	c := NewCache(context.Background())
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "ran")

	_, err := c.probe(context.Background(), sentinelDescriptor(sentinel), models.ProbeOptions{Cwd: dir}, nil)

	require.ErrorIs(t, err, errProbingDisabledUnderTest)
	_, statErr := os.Stat(sentinel)
	assert.True(t, os.IsNotExist(statErr))
}
