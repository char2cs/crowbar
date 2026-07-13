package agent_test

import (
	"os"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return strings.TrimPrefix(kv, prefix)
		}
	}
	return ""
}

func TestBuildSpawnPlan_ClaudeWritesSettingsAndArgs(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar", Segid: "seg-9", Provider: "claude"}

	plan, err := agent.BuildSpawnPlan(d, ctx, os.Environ(), nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	// --settings <file> present and the file exists with a hook command
	require.Contains(t, plan.Argv, "--settings")
	idx := indexOf(plan.Argv, "--settings")
	settingsPath := plan.Argv[idx+1]
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "SessionStart")
	require.Contains(t, string(data), "/bin/crowbar hook turn_stop --segment seg-9 --provider claude")

	// nested-CC markers are cleared from Env
	for _, kv := range plan.Env {
		require.False(t, strings.HasPrefix(kv, "CLAUDE_CODE_CHILD_SESSION="))
		require.False(t, strings.HasPrefix(kv, "CLAUDECODE="))
	}
}

func TestBuildSpawnPlan_CodexInjectsHooksWithoutOwningItsHome(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	tmp := t.TempDir()
	ctx := agent.TemplateCtx{
		Tmp: tmp, Cwd: t.TempDir(),
		CrowbarHook: "/bin/crowbar", Segid: "seg-c", Provider: "codex",
	}
	plan, err := agent.BuildSpawnPlan(d, ctx, os.Environ(), nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	require.Contains(t, plan.Argv, "--dangerously-bypass-hook-trust")

	// Crowbar must NOT own codex's home. It used to point CODEX_HOME at a directory
	// it created and deleted, which made it the custodian of codex's SESSIONS — and
	// it duly destroyed them on every provider switch. A provider owns its own
	// state; Crowbar injects its hooks as config overrides and writes nothing.
	assert.Empty(t, envValue(plan.Env, "CODEX_HOME"),
		"codex must run against the user's real ~/.codex — its sessions, skills and MCP servers live there")

	joined := strings.Join(plan.Argv, "\x00")
	assert.Contains(t, joined, `hooks.SessionStart=`)
	assert.Contains(t, joined, `hooks.UserPromptSubmit=`)
	assert.Contains(t, joined, `hooks.Stop=`)
	assert.Contains(t, joined, "--segment seg-c --provider codex",
		"the segment id travels inside the hook command, so nothing has to be written to disk")

	// Nothing at all was written into the per-spawn tmp dir.
	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries, "the codex descriptor must write no files")
}

func TestBuildSpawnPlan_RejectsForbiddenFlag(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	// A handoff/positional step that smuggles a headless flag must be rejected.
	_, err = agent.BuildSpawnPlan(d, ctx, os.Environ(), []agent.InjectStep{
		{Verb: "pass_arg", Args: map[string]any{"positional": "-p"}},
	})
	require.Error(t, err)
}
