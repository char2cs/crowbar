package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
