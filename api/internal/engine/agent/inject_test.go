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

func TestBuildSpawnPlan_CodexSetsHomeAndBypassFlag(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	tmp, chatDir := t.TempDir(), t.TempDir()
	ctx := agent.TemplateCtx{
		Tmp: tmp, ChatDir: chatDir, Cwd: t.TempDir(),
		CrowbarHook: "/bin/crowbar", Segid: "seg-c", Provider: "codex",
	}
	plan, err := agent.BuildSpawnPlan(d, ctx, os.Environ(), nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	require.Contains(t, plan.Argv, "--dangerously-bypass-hook-trust")

	// CODEX_HOME must sit under the CHAT dir, never under the per-segment tmp dir.
	// codex's resumable session rollouts live inside it, and tmp is deleted the
	// moment the segment's CLI exits — which destroyed codex's own session and made
	// switching BACK to codex kill it on startup ("no rollout found for thread id").
	home := envValue(plan.Env, "CODEX_HOME")
	require.Contains(t, home, filepath.Base(chatDir))
	require.NotContains(t, home, filepath.Base(tmp))

	hooksData, err := os.ReadFile(home + "/hooks.json")
	require.NoError(t, err)
	require.Contains(t, string(hooksData), "--segment seg-c --provider codex")
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
