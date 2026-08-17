package spawn_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func passArg(args map[string]any) spec.InjectStep {
	return spec.InjectStep{Verb: "pass_arg", Args: args}
}

func base(t *testing.T) (*spec.Descriptor, models.TemplateCtx) {
	t.Helper()
	d := &spec.Descriptor{ID: "probe"}
	d.Spawn.Cmd = "probe-cli"
	d.Spawn.InteractiveRequired = true
	return d, models.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir()}
}

func TestPlan_RendersStaticArgsFirst(t *testing.T) {
	d, ctx := base(t)
	d.Spawn.Args = []string{"--sandbox", "workspace-write"}

	plan, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"--sandbox", "workspace-write"}, plan.Argv)
	assert.Equal(t, ctx.Cwd, plan.Cwd)
	assert.Equal(t, ctx.Tmp, plan.TmpDir)
	assert.NotEmpty(t, plan.Executable, "a plan the caller cannot exec is not a plan")
}

// Claude's --mcp-config is VARIADIC and swallows any bare positional after it.
// What stops that is the config step sitting immediately behind it, which only
// holds if MCP steps are rendered BEFORE config steps.
func TestPlan_RendersMCPStepsBeforeConfigStepsBeforeExtras(t *testing.T) {
	d, ctx := base(t)
	d.MCPInject = []spec.InjectStep{passArg(map[string]any{"arg": "--mcp-config", "value": "{}"})}
	d.ConfigInjection = []spec.InjectStep{passArg(map[string]any{"arg": "--settings", "value": "s.json"})}
	extra := []spec.InjectStep{passArg(map[string]any{"positional": "--"})}

	plan, err := spawn.Plan(d, ctx, nil, extra)

	require.NoError(t, err)
	assert.Equal(t, []string{"--mcp-config", "{}", "--settings", "s.json", "--"}, plan.Argv)
}

func TestPlan_ExpandsPlaceholdersInEveryPosition(t *testing.T) {
	d, ctx := base(t)
	ctx.Segid = "SEG"
	ctx.Message = "hello world"
	d.Spawn.Args = []string{"--segment", "{segid}"}
	d.ConfigInjection = []spec.InjectStep{passArg(map[string]any{"positional": "{message}"})}

	plan, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"--segment", "SEG", "hello world"}, plan.Argv,
		"a message with spaces must remain ONE argv element")
}

func TestPlan_ClearsTheDeclaredEnvironmentMarkers(t *testing.T) {
	d, ctx := base(t)
	d.Spawn.Env.Clear = []string{"CLAUDECODE"}

	plan, err := spawn.Plan(d, ctx, []string{"A=1", "CLAUDECODE=1"}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"A=1"}, plan.Env)
}

func TestPlan_SetEnvAppendsToTheChildEnvironment(t *testing.T) {
	d, ctx := base(t)
	d.ConfigInjection = []spec.InjectStep{
		{Verb: "set_env", Args: map[string]any{"name": "CROWBAR_SEG", "value": "{segid}"}},
	}
	ctx.Segid = "SEG"

	plan, err := spawn.Plan(d, ctx, []string{"A=1"}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"A=1", "CROWBAR_SEG=SEG"}, plan.Env)
}

func TestPlan_WriteFileMaterialisesConfigUnderTheSpawnDir(t *testing.T) {
	d, ctx := base(t)
	d.ConfigInjection = []spec.InjectStep{
		{Verb: "write_file", Args: map[string]any{
			"path": filepath.Join(ctx.Tmp, "nested", "settings.json"), "content": `{"seg":"{segid}"}`,
		}},
	}
	ctx.Segid = "SEG"

	_, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	body, readErr := os.ReadFile(filepath.Join(ctx.Tmp, "nested", "settings.json"))
	require.NoError(t, readErr)
	assert.JSONEq(t, `{"seg":"SEG"}`, string(body))
}

// The source is optional config; refusing to start a CLI because an optional file
// is absent trades a degraded session for none at all.
func TestPlan_WriteFileWithAMissingSourceWritesAnEmptyDestination(t *testing.T) {
	d, ctx := base(t)
	dst := filepath.Join(ctx.Tmp, "copied.json")
	d.ConfigInjection = []spec.InjectStep{
		{Verb: "write_file", Args: map[string]any{"path": dst, "from": filepath.Join(t.TempDir(), "gone")}},
	}

	_, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	body, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Empty(t, body)
}

func TestPlan_WriteFileCopiesAnExistingSource(t *testing.T) {
	d, ctx := base(t)
	src := filepath.Join(t.TempDir(), "src.json")
	require.NoError(t, os.WriteFile(src, []byte("original"), 0o600))
	dst := filepath.Join(ctx.Tmp, "copied.json")
	d.ConfigInjection = []spec.InjectStep{
		{Verb: "write_file", Args: map[string]any{"path": dst, "from": src}},
	}

	_, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	body, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(body))
}

func TestPlan_WriteFileFailureCleansUpAndReports(t *testing.T) {
	d, ctx := base(t)
	blocker := filepath.Join(ctx.Tmp, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	d.ConfigInjection = []spec.InjectStep{
		{Verb: "write_file", Args: map[string]any{"path": filepath.Join(blocker, "x.json"), "content": "{}"}},
	}

	_, err := spawn.Plan(d, ctx, nil, nil)

	require.Error(t, err)
	_, statErr := os.Stat(ctx.Tmp)
	assert.True(t, os.IsNotExist(statErr), "a failed plan must not leave its spawn dir behind")
}

// A silently ignored verb spawns a CLI missing the config its author believed it
// had.
func TestPlan_AnUnknownVerbIsRefused(t *testing.T) {
	d, ctx := base(t)
	d.ConfigInjection = []spec.InjectStep{{Verb: "telepathy", Args: map[string]any{}}}

	_, err := spawn.Plan(d, ctx, nil, nil)

	assert.Error(t, err)
}

// A flag whose value is legitimately empty still needs its own argv slot, or the
// next token silently becomes its value.
func TestPlan_PassArgEmitsAPresentButEmptyValueAsItsOwnToken(t *testing.T) {
	d, ctx := base(t)
	d.ConfigInjection = []spec.InjectStep{
		passArg(map[string]any{"arg": "--repo", "value": ""}),
		passArg(map[string]any{"arg": "--workspace", "value": "w"}),
	}

	plan, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"--repo", "", "--workspace", "w"}, plan.Argv)
}

func TestPlan_PassArgWithNoValueKeyEmitsOnlyTheFlag(t *testing.T) {
	d, ctx := base(t)
	d.ConfigInjection = []spec.InjectStep{passArg(map[string]any{"arg": "--verbose"})}

	plan, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"--verbose"}, plan.Argv)
}

// The hard guard: the engine must never spawn a headless CLI.
func TestPlan_RefusesAForbiddenFlag(t *testing.T) {
	d, ctx := base(t)
	d.Spawn.ForbidFlags = []string{"-p", "--print"}
	d.ConfigInjection = []spec.InjectStep{passArg(map[string]any{"arg": "--print"})}

	_, err := spawn.Plan(d, ctx, nil, nil)

	require.ErrorIs(t, err, spawn.ErrForbiddenFlag)
	_, statErr := os.Stat(ctx.Tmp)
	assert.True(t, os.IsNotExist(statErr))
}

// After an end-of-options marker everything is DATA. A user whose prompt happens
// to be the exact text "--print" must not have their message read as a flag.
func TestPlan_AForbiddenStringAfterEndOfOptionsIsDataNotAFlag(t *testing.T) {
	d, ctx := base(t)
	d.Spawn.ForbidFlags = []string{"--print"}
	d.ConfigInjection = []spec.InjectStep{
		passArg(map[string]any{"positional": "--"}),
		passArg(map[string]any{"positional": "--print"}),
	}

	plan, err := spawn.Plan(d, ctx, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"--", "--print"}, plan.Argv)
}

func TestPromptSteps_ReturnsADefensiveCopy(t *testing.T) {
	d, _ := base(t)
	d.Presentation.PromptSubmit = &spec.PromptSubmitSpec{
		Fresh:  []spec.InjectStep{passArg(map[string]any{"positional": "{message}"})},
		Resume: []spec.InjectStep{passArg(map[string]any{"positional": "resumed {message}"})},
	}

	fresh, ok := spawn.PromptSteps(d, false)
	require.True(t, ok)
	fresh[0].Args["positional"] = "MUTATED"

	again, ok := spawn.PromptSteps(d, false)
	require.True(t, ok)
	assert.Equal(t, "{message}", again[0].Args["positional"],
		"a caller must not be able to rewrite the descriptor through the steps it was handed")

	resume, ok := spawn.PromptSteps(d, true)
	require.True(t, ok)
	assert.Equal(t, "resumed {message}", resume[0].Args["positional"])
}

func TestPromptSteps_AbsentCapabilityIsReportedAsAbsent(t *testing.T) {
	d, _ := base(t)

	_, ok := spawn.PromptSteps(d, false)
	assert.False(t, ok)

	_, ok = spawn.PromptSteps(nil, false)
	assert.False(t, ok)
}
