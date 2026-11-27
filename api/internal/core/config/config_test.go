package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrompts_FromEmbeddedDefaults(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	resetForTesting()
	t.Cleanup(resetForTesting)

	p := GetPrompts()
	assert.Contains(t, p.HandoffWrapper, "{conversation}")
	assert.Contains(t, p.HandoffResumeWrapper, "{conversation}")
	assert.Contains(t, p.HandoffPointer, "{chat_id}")

	assert.Contains(t, p.ThreadLineage, "{lineage}")
	assert.Contains(t, p.ThreadLineage, "get_chat_log")
	assert.NotContains(t, p.ThreadLineage, "{conversation}")

	for name, text := range map[string]string{
		"capabilities_instruction": p.CapabilitiesInstruction,
		"handoff_wrapper":          p.HandoffWrapper,
		"handoff_resume_wrapper":   p.HandoffResumeWrapper,
		"handoff_pointer":          p.HandoffPointer,
		"thread_lineage":           p.ThreadLineage,
	} {
		assert.NotContains(t, text, "{crowbar}", "%s must not ask the agent to shell out", name)
	}

	assert.Contains(t, p.CapabilitiesInstruction, "Crowbar workspace")
	assert.Contains(t, p.CapabilitiesInstruction, "prefer them over shell equivalents")

	assert.Contains(t, p.CapabilitiesInstruction, "set_chat_title")
	assert.NotContains(t, p.CapabilitiesInstruction, "chat rename")

	assert.Contains(t, p.CapabilitiesInstruction, "review")
	assert.Contains(t, p.CapabilitiesInstruction, "may be")
	assert.NotContains(t, p.CapabilitiesInstruction, "GitHub")
	assert.NotContains(t, p.CapabilitiesInstruction, "gh pr")
	assert.NotContains(t, p.CapabilitiesInstruction, "only")

	assert.Contains(t, p.CapabilitiesInstruction, "wait_for_review_comments")
	assert.Contains(t, p.CapabilitiesInstruction, "list_review_threads")
	for _, imperative := range []string{"must", "always", "loop", "poll"} {
		assert.NotContains(t, p.CapabilitiesInstruction, imperative)
	}
}

func TestGetPrompts_UserConfigOverlays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CROWBAR_HOME", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("config:\n  prompts:\n    handoff_wrapper: \"CUSTOM {conversation}\"\n"), 0o644))
	resetForTesting()

	p := GetPrompts()
	assert.Equal(t, "CUSTOM {conversation}", p.HandoffWrapper)
	assert.Contains(t, p.HandoffPointer, "{chat_id}")
}

func TestGetPrompts_UnresolvableHomeKeepsDefaults(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "relative/home")
	resetForTesting()
	t.Cleanup(resetForTesting)

	assert.Contains(t, GetPrompts().HandoffPointer, "{chat_id}")
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
