package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrompts_FromEmbeddedDefaults(t *testing.T) {
	// Isolate from a dev box's real ~/.crowbar/config.yaml so this test only
	// ever sees the embedded defaults.
	t.Setenv("CROWBAR_HOME", t.TempDir())
	resetForTesting()
	t.Cleanup(resetForTesting)

	p := GetPrompts()
	// {scope_flags} — NOT a literal --project/--repo/--workspace triple. The agent
	// retypes this command line and the shell word-splits it, so an empty repo id
	// (every project-home workspace) must render as NO --repo flag at all rather
	// than a bare `--repo `, which the shell drops and pflag then backfills from the
	// next token. See engine/agent.TemplateCtx.ScopeFlags.
	assert.Contains(t, p.TitleInstruction, "chat rename {scope_flags} --segment {segid}")
	assert.Contains(t, p.HandoffWrapper, "{conversation}")

	// The capability preamble is a DIRECTIVE, not a tool list: registering the MCP
	// server puts the tools in the model's list, but nothing tells it to reach for them
	// over the shell equivalents it already knows.
	assert.Contains(t, p.CapabilitiesInstruction, "Crowbar workspace")
	assert.Contains(t, p.CapabilitiesInstruction, "prefer them over shell equivalents")

	// And it must name no capability that is not registered. The only tool on the
	// surface at this phase is set_chat_title, so the text stays generic: a directive
	// pointing at an absent tool family, while forbidding the fallback the model would
	// otherwise reach for, is worse than no directive at all. The review directive
	// lands with the review tools.
	assert.NotContains(t, p.CapabilitiesInstruction, "review")
	assert.NotContains(t, p.CapabilitiesInstruction, "gh pr")
}

func TestGetPrompts_UserConfigOverlays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CROWBAR_HOME", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("config:\n  prompts:\n    handoff_wrapper: \"CUSTOM {conversation}\"\n"), 0o644))
	// metadata.resolveHome() reads CROWBAR_HOME straight from os.Getenv on
	// every call (see resolve_home.go) and isn't cached anywhere, so no
	// metadata-side reset is needed to pick up the new CROWBAR_HOME here.
	resetForTesting()

	p := GetPrompts()
	assert.Equal(t, "CUSTOM {conversation}", p.HandoffWrapper)
	// absent field keeps the embedded default
	assert.Contains(t, p.TitleInstruction, "chat rename {scope_flags} --segment {segid}")
}
