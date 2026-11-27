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

func TestGuardedProbe_LiveEnvVarOptsBackIntoARealFork(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "ran")
	t.Setenv(liveProbeEnvVar, "1")

	_, err := guardedProbe(context.Background(), sentinelDescriptor(sentinel), models.ProbeOptions{Cwd: dir}, nil)

	require.NoError(t, err)
	_, statErr := os.Stat(sentinel)
	assert.NoError(t, statErr, "the escape hatch must actually run the real probe")
}

func TestNewCache_DefaultProbeIsGuarded(t *testing.T) {
	c := NewCache(context.Background())
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "ran")

	_, err := c.probe(context.Background(), sentinelDescriptor(sentinel), models.ProbeOptions{Cwd: dir}, nil)

	require.ErrorIs(t, err, errProbingDisabledUnderTest)
	_, statErr := os.Stat(sentinel)
	assert.True(t, os.IsNotExist(statErr))
}
