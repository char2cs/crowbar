package agents_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func get(t *testing.T, id string) agents.Agent {
	t.Helper()
	a, err := agents.New().Get(context.Background(), "", id)
	require.NoError(t, err)
	return a
}

func TestList_EnumeratesTheShippedAgents(t *testing.T) {
	list, err := agents.New().List(context.Background(), "")

	require.NoError(t, err)
	ids := make([]string, 0, len(list))
	for _, a := range list {
		ids = append(ids, a.ID())
	}
	assert.Equal(t, []string{"claude", "codex"}, ids)
}

func TestGet_UnknownAgentIsReported(t *testing.T) {
	_, err := agents.New().Get(context.Background(), "", "telepathy")

	assert.ErrorIs(t, err, agents.ErrUnknownAgent)
}

func TestAgent_ReportsItsIdentityAndDisplay(t *testing.T) {
	a := get(t, "claude")

	assert.Equal(t, "claude", a.ID())
	assert.Equal(t, "Claude", a.Display().Name)
	assert.NotEmpty(t, a.Display().Icon)
}

// An absent capability renders as absent UI, never as a disabled control
// implying breakage — so the capability report must be a fact about the
// descriptor rather than a guess.
func TestAgent_CapabilitiesReportWhatTheDescriptorDeclares(t *testing.T) {
	claude := get(t, "claude").Capabilities()
	assert.True(t, claude.PromptSubmit)
	assert.Equal(t, agents.DeliveryRestartTUI, claude.Delivery)
	assert.True(t, claude.SlashCatalog)
	assert.True(t, claude.Telemetry)
	assert.True(t, claude.Declares(agents.HookToolPre))
	assert.True(t, claude.Declares(agents.HookNotification))

	codex := get(t, "codex").Capabilities()
	assert.True(t, codex.PromptSubmit)
	assert.False(t, codex.Telemetry, "codex exposes no telemetry channel today")
	assert.False(t, codex.Declares(agents.HookNotification), "codex has no Notification event")
	assert.True(t, codex.Declares(agents.HookPermission))
	assert.False(t, codex.Declares("no_such_kind"))
}

func TestAgent_SpawnPlanRendersAnExecutableLaunch(t *testing.T) {
	a := get(t, "claude")
	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), Segid: "SEG", Provider: "claude",
		ProjectID: "P", WorkspaceID: "W", CrowbarHook: "/bin/crowbar",
	}

	plan, err := a.SpawnPlan(ctx, os.Environ(), nil)

	require.NoError(t, err)
	assert.NotEmpty(t, plan.Executable)
	assert.Contains(t, plan.Argv, "--permission-mode")
	assert.Equal(t, ctx.Cwd, plan.Cwd)
	require.NotNil(t, plan.Cleanup)
	plan.Cleanup()
	_, statErr := os.Stat(ctx.Tmp)
	assert.True(t, os.IsNotExist(statErr))
}

// The tool surface is a per-provider preference. Turning it off must not mutate a
// descriptor other spawns may share, or one chat's choice becomes every later
// chat's.
func TestAgent_WithToolsCopiesRatherThanMutating(t *testing.T) {
	a := get(t, "claude")
	ctx := func() agents.TemplateCtx {
		return agents.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), Segid: "SEG"}
	}

	withTools, err := a.SpawnPlan(ctx(), nil, nil)
	require.NoError(t, err)
	require.Contains(t, strings.Join(withTools.Argv, " "), "--mcp-config")

	off, err := a.WithTools(false).SpawnPlan(ctx(), nil, nil)
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(off.Argv, " "), "--mcp-config")

	again, err := a.SpawnPlan(ctx(), nil, nil)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(again.Argv, " "), "--mcp-config",
		"switching the tool surface off for one spawn must not disable it for the next")

	assert.Same(t, a, a.WithTools(true), "leaving it on needs no copy")
}

func TestAgent_PromptStepsPlaceTheMessageExactlyOnceAfterEndOfOptions(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			a := get(t, id)
			for _, resume := range []bool{false, true} {
				steps, err := a.PromptSteps(resume)
				require.NoError(t, err)

				plan, err := a.SpawnPlan(agents.TemplateCtx{
					Tmp: t.TempDir(), Cwd: t.TempDir(), Message: "--print",
				}, nil, steps)
				require.NoError(t, err)

				end := indexOf(plan.Argv, "--")
				require.GreaterOrEqual(t, end, 0, "argv %v must end its options", plan.Argv)
				assert.Equal(t, "--print", plan.Argv[len(plan.Argv)-1],
					"a message that looks like a forbidden flag must survive as DATA")
			}
		})
	}
}

func TestAgent_ContextStepsDifferBetweenFreshAndResume(t *testing.T) {
	codex := get(t, "codex")

	fresh := codex.ContextSteps(false)
	resumed := codex.ContextSteps(true)

	require.NotEmpty(t, fresh)
	require.NotEmpty(t, resumed)
	// A resumed codex ignores every config channel; only a user message reaches it.
	assert.Contains(t, fresh[0].Args, "arg")
	assert.Contains(t, resumed[0].Args, "positional")
}

func TestAgent_ContextStepsAreADefensiveCopy(t *testing.T) {
	a := get(t, "claude")

	first := a.ContextSteps(false)
	require.NotEmpty(t, first)
	first[0].Args["arg"] = "MUTATED"

	assert.NotEqual(t, "MUTATED", a.ContextSteps(false)[0].Args["arg"])
}

func TestAgent_ResumeArgIsDeclaredByBothShippedAgents(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		arg, ok := get(t, id).ResumeArg()
		assert.True(t, ok, id)
		assert.Contains(t, arg, "{id}", id)
	}
}

func TestAgent_ParseHookMapsAConversationTurn(t *testing.T) {
	a := get(t, "claude")

	ev, err := a.ParseHook(agents.HookTurnStop,
		[]byte(`{"session_id":"s1","last_assistant_message":"done","background_tasks":[1]}`))

	require.NoError(t, err)
	assert.Equal(t, "s1", ev.SessionID)
	assert.Equal(t, "done", ev.Message)
	assert.Equal(t, 1, ev.AsyncWork)
}

// The ownership guard is fused into ParseHook so a caller cannot skip it.
func TestAgent_ParseHookRefusesAnotherConversationsPayload(t *testing.T) {
	a := get(t, "codex")

	_, err := a.ParseHook(agents.HookUserPrompt, []byte(`{"prompt":"x","transcript_path":null}`))

	assert.ErrorIs(t, err, agents.ErrForeignConversation)
}

func TestAgent_ParseHookReportsAnUndeclaredEvent(t *testing.T) {
	_, err := get(t, "codex").ParseHook(agents.HookNotification,
		[]byte(`{"transcript_path":"/x","message":"hi"}`))

	assert.ErrorIs(t, err, agents.ErrHookUndeclared)
}

func TestAgent_ParseTelemetryMapsTheProvidersReport(t *testing.T) {
	a := get(t, "claude")
	now := time.Now()

	got, err := a.ParseTelemetry([]byte(`{
		"context_window":{"context_window_size":200000,"used_percentage":19},
		"model":{"id":"m","display_name":"M"}}`), now)

	require.NoError(t, err)
	require.NotNil(t, got.Context)
	assert.Equal(t, 200000, *got.Context.CapacityTokens)
	require.NotNil(t, got.Model)
	assert.Equal(t, "m", got.Model.ID)
	assert.Equal(t, agents.TelemetrySourceCallback, got.Source)
	assert.Equal(t, now, got.ObservedAt)
}

func TestAgent_ParseTelemetryIsUnsupportedWhereNoChannelIsDeclared(t *testing.T) {
	_, err := get(t, "codex").ParseTelemetry([]byte(`{}`), time.Now())

	assert.ErrorIs(t, err, agents.ErrTelemetryUnsupported)
}

func TestAgent_SlashCatalogRefusesAnInvalidWorkdir(t *testing.T) {
	_, err := get(t, "claude").SlashCatalog(context.Background(), agents.ProbeOptions{Cwd: "relative"}, nil)

	assert.ErrorIs(t, err, agents.ErrCatalogInvalidWorkdir)
}

func TestExpand_RendersCrowbarsOwnPrompts(t *testing.T) {
	got := agents.Expand("chat {chat_id} limit {gap_turns}",
		agents.TemplateCtx{ChatID: "c1", GapTurns: "4"})

	assert.Equal(t, "chat c1 limit 4", got)
}

func TestDecide_IsReExportedAsAPureFunction(t *testing.T) {
	assert.Equal(t, agents.MoveNoop, agents.Decide("s1", "s1", "", false).Kind)
	assert.Equal(t, agents.MoveBind, agents.Decide("", "s1", "", false).Kind)
	assert.Equal(t, agents.MoveToNew, agents.Decide("s1", "s2", "", false).Kind)

	known := agents.Decide("s1", "s2", "chat-9", true)
	assert.Equal(t, agents.MoveToKnown, known.Kind)
	assert.Equal(t, "chat-9", known.ChatID)
}

func TestInjectionRegistry_RecognisesAnEchoOncePerRunner(t *testing.T) {
	e := agents.New()
	e.RecordInjection("runner-1", "handoff blob")

	assert.True(t, e.WasInjected("runner-1", "handoff blob"))
	assert.False(t, e.WasInjected("runner-1", "handoff blob"))

	e.RecordInjection("runner-2", "other")
	e.ForgetRunner("runner-2")
	assert.False(t, e.WasInjected("runner-2", "other"))
}

// Every shipped descriptor must render MCP registration that a provider can
// actually parse, and claude's variadic --mcp-config must never be followed by a
// bare positional.
func TestShippedAgents_RenderParseableMCPRegistration(t *testing.T) {
	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), Segid: "SEG", RunnerToken: "TOK",
		ProjectID: "P", RepoID: "R", WorkspaceID: "W", CrowbarHook: "/bin/crowbar",
	}

	claude, err := get(t, "claude").SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)
	cfgIdx := indexOf(claude.Argv, "--mcp-config")
	require.GreaterOrEqual(t, cfgIdx, 0)
	require.Less(t, cfgIdx+2, len(claude.Argv), "the JSON must be followed by another FLAG")
	assert.True(t, strings.HasPrefix(claude.Argv[cfgIdx+2], "-"),
		"a bare positional after a variadic --mcp-config is swallowed as another config")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(claude.Argv[cfgIdx+1]), &parsed))
	assert.Contains(t, parsed, "mcpServers")

	codex, err := get(t, "codex").SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(codex.Argv, " "), "mcp_servers.crowbar.command")
}

// A project-home workspace has NO repo id, and the rendered hook command is a
// flat shell string with no quoting.
func TestShippedAgents_RenderHookCommandsThatSurviveAnEmptyRepoID(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			tmp := t.TempDir()
			plan, err := get(t, id).SpawnPlan(agents.TemplateCtx{
				Tmp: tmp, Cwd: t.TempDir(), Segid: "SEG", Provider: id,
				ProjectID: "P", RepoID: "", WorkspaceID: "W", CrowbarHook: "/bin/crowbar",
			}, nil, nil)
			require.NoError(t, err)

			rendered := strings.Join(plan.Argv, " ") + " " + readAll(t, tmp)
			assert.Contains(t, rendered, "--project=P")
			assert.Contains(t, rendered, "--workspace=W")
			assert.NotContains(t, rendered, "--repo=",
				"an absent repo id must omit the flag, never leave a danging one")
		})
	}
}

func readAll(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, readErr)
		b.Write(data)
	}
	return b.String()
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

func TestAgent_InstalledReportsWhetherTheCLIIsPresent(t *testing.T) {
	// Whatever the host has, the answer must be a definite boolean rather than a
	// panic or a guess — that is the whole contract.
	assert.NotPanics(t, func() { _ = get(t, "claude").Installed() })

	sh := stubAgent(t, "sh")
	assert.True(t, sh.Installed())

	missing := stubAgent(t, "crowbar-definitely-not-installed-xyz")
	assert.False(t, missing.Installed())
}

func TestAgent_PromptStepsAreUnsupportedWhereNoneAreDeclared(t *testing.T) {
	a := stubAgent(t, "sh")

	_, err := a.PromptSteps(false)

	assert.ErrorIs(t, err, agents.ErrPromptSubmitUnsupported)
}

func TestAgent_ResumeArgIsAbsentWhereNoneIsDeclared(t *testing.T) {
	_, ok := stubAgent(t, "sh").ResumeArg()

	assert.False(t, ok)
}

func TestAgent_ProbeTelemetryIsUnsupportedForBothShippedAgents(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		_, err := get(t, id).ProbeTelemetry(
			context.Background(), agents.ProbeOptions{Cwd: t.TempDir()}, nil, time.Now(),
		)
		assert.ErrorIs(t, err, agents.ErrTelemetryUnsupported, id)
	}
}

func TestAgent_ProbeTelemetryRunsADeclaredCommand(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "polled", `
id: polled
spawn:
  cmd: sh
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop:     { message: last }
telemetry:
  probe:
    format: json
    command: ["-c", "cat testdata/probe_telemetry.json"]
    fields:
      context.capacity_tokens: cap
`)
	a, err := agents.New().Get(context.Background(), home, "polled")
	require.NoError(t, err)

	// The probe runs in the chat's worktree, so the fixture is read relative to it.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	got, err := a.ProbeTelemetry(context.Background(),
		agents.ProbeOptions{Cwd: cwd, Env: os.Environ()}, nil, time.Now())

	require.NoError(t, err)
	require.NotNil(t, got.Context)
	assert.Equal(t, 4096, *got.Context.CapacityTokens)
	assert.Equal(t, agents.TelemetrySourceProbe, got.Source)
}

func TestAgent_ProbeTelemetryRefusesAnInvalidWorkdir(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "polled", `
id: polled
spawn:
  cmd: sh
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop:     { message: last }
telemetry:
  probe:
    format: json
    command: ["-c", "true"]
    fields:
      context.capacity_tokens: cap
`)
	a, err := agents.New().Get(context.Background(), home, "polled")
	require.NoError(t, err)

	_, err = a.ProbeTelemetry(context.Background(), agents.ProbeOptions{Cwd: "relative"}, nil, time.Now())

	assert.ErrorIs(t, err, agents.ErrTelemetryInvalidWorkdir)
}

// stubAgent resolves a minimal on-disk descriptor: enough to exist, and nothing
// else declared, so absent capabilities can be asserted as absent.
func stubAgent(t *testing.T, cmd string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	writeDescriptor(t, home, "stub", `
id: stub
spawn:
  cmd: `+cmd+`
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop:     { message: last }
`)
	a, err := agents.New().Get(context.Background(), home, "stub")
	require.NoError(t, err)
	return a
}

func writeDescriptor(t *testing.T, home, id, body string) {
	t.Helper()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o600))
}
