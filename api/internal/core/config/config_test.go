package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelForTier_KnownTiers(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.Equal(t, "claude-haiku-4-5", ModelForTier("light"))
	assert.Equal(t, "claude-sonnet-4-6", ModelForTier("medium"))
	assert.Equal(t, "claude-opus-4-8", ModelForTier("heavy"))
}

func TestModelForTier_UnknownTier_ReturnsEmpty(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.Equal(t, "", ModelForTier("nonexistent"))
}

func TestGetIntelligence_FromEmbeddedDefaults(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	i := GetIntelligence()
	assert.NotEmpty(t, i.Medium)
}

func TestGetDefaultConfig_ReturnsDefaults(t *testing.T) {
	cfg := getDefaultConfig()
	assert.Equal(t, "claude-sonnet-4-6", cfg.Config.Intelligence.Medium)
	assert.Equal(t, "claude-haiku-4-5", cfg.Config.Intelligence.Light)
	assert.Equal(t, "claude-opus-4-8", cfg.Config.Intelligence.Heavy)
}

func TestGet_OverlayFromUserConfigFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Cleanup(resetForTesting)
	resetForTesting()

	crowbarDir := filepath.Join(tmp, ".crowbar")
	require.NoError(t, os.MkdirAll(crowbarDir, 0o750))

	userConfig := "config:\n  intelligence:\n    medium: \"claude-custom-model\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(crowbarDir, "config.yaml"), []byte(userConfig), 0o600))

	got := ModelForTier("medium")
	assert.Equal(t, "claude-custom-model", got)
}

func TestGet_MalformedYAML_FallsBackToDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Cleanup(resetForTesting)
	resetForTesting()

	crowbarDir := filepath.Join(tmp, ".crowbar")
	require.NoError(t, os.MkdirAll(crowbarDir, 0o750))

	require.NoError(t, os.WriteFile(filepath.Join(crowbarDir, "config.yaml"), []byte(":::bad yaml:::"), 0o600))

	cfg := Get()
	// Falls back to defaults when YAML is malformed.
	assert.Equal(t, "claude-sonnet-4-6", cfg.Config.Intelligence.Medium)
}
