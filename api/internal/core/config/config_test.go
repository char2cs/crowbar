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
	assert.Contains(t, p.HandoffWrapper, "{conversation}")
	assert.Contains(t, p.HandoffResumeWrapper, "{conversation}")
	assert.Contains(t, p.HandoffPointer, "{ledger_dir}")

	// A thread is handed IDS and told to fetch the conversations itself. The
	// placeholder is the whole difference between a thread and a fork: a prompt
	// that carried the ancestors' turns instead would freeze them at the instant
	// the thread spawned, and nothing could ever refresh them.
	assert.Contains(t, p.ThreadLineage, "{lineage}")
	assert.Contains(t, p.ThreadLineage, "get_chat_log")
	assert.NotContains(t, p.ThreadLineage, "{conversation}")

	// No prompt may ask the agent to run a `crowbar` command. Titling used to be
	// exactly that, and it COMPETED with the set_chat_title tool: handed both, a
	// model would read the tool list and then type the shell command instead. Every
	// capability is a tool now; prompts carry context only.
	for name, text := range map[string]string{
		"capabilities_instruction": p.CapabilitiesInstruction,
		"handoff_wrapper":          p.HandoffWrapper,
		"handoff_resume_wrapper":   p.HandoffResumeWrapper,
		"handoff_pointer":          p.HandoffPointer,
		"thread_lineage":           p.ThreadLineage,
	} {
		assert.NotContains(t, text, "{crowbar}", "%s must not ask the agent to shell out", name)
	}

	// The capability preamble is a DIRECTIVE, not a tool list: registering the MCP
	// server puts the tools in the model's list, but nothing tells it to reach for them
	// over the shell equivalents it already knows.
	assert.Contains(t, p.CapabilitiesInstruction, "Crowbar workspace")
	assert.Contains(t, p.CapabilitiesInstruction, "prefer them over shell equivalents")

	// Titling must be ASKED FOR, not merely made possible. Retiring the old
	// title_instruction removed the shell command AND the request along with it,
	// leaving set_chat_title registered but nothing telling a model to call it —
	// so neither provider titled a chat at all. A tool description states a
	// capability; only the preamble states an expectation, and titling is
	// proactive work unrelated to whatever the user actually asked for, so it
	// does not happen unless something asks.
	//
	// It names the TOOL, never a shell command, which is what keeps it from
	// re-creating the competition that retiring title_instruction removed.
	assert.Contains(t, p.CapabilitiesInstruction, "set_chat_title")
	assert.NotContains(t, p.CapabilitiesInstruction, "chat rename")

	// The review tools now exist (Task 13), so the preamble carries the directive
	// that was held back until they did: prefer Crowbar's own review surface over
	// `gh pr`, and post findings as anchored threads rather than chat prose.
	// TestCapabilitiesPreamble_OnlyNamesRegisteredTools (agenttools package) is the
	// tripwire that keeps this claim honest against the actual tool registry.
	assert.Contains(t, p.CapabilitiesInstruction, "review")
	assert.Contains(t, p.CapabilitiesInstruction, "gh pr")
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
	assert.Contains(t, p.HandoffPointer, "{ledger_dir}")
}
