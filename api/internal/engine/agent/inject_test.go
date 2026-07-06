package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
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
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}

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
	require.Contains(t, string(data), "/bin/crowbar")

	// nested-CC markers are cleared from Env
	for _, kv := range plan.Env {
		require.False(t, strings.HasPrefix(kv, "CLAUDE_CODE_CHILD_SESSION="))
		require.False(t, strings.HasPrefix(kv, "CLAUDECODE="))
	}
}

func TestBuildSpawnPlan_CodexSetsHomeAndBypassFlag(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	plan, err := agent.BuildSpawnPlan(d, ctx, os.Environ(), nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	require.Contains(t, plan.Argv, "--dangerously-bypass-hook-trust")
	require.Contains(t, envValue(plan.Env, "CODEX_HOME"), filepath.Base(ctx.Tmp)) // CODEX_HOME under tmp
	_, err = os.Stat(envValue(plan.Env, "CODEX_HOME") + "/hooks.json")
	require.NoError(t, err)
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
