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
	// claude declares the rewake channel; codex below is the portable floor. The
	// pair is what makes this assertion a fact about descriptors rather than a
	// constant repeated in Go.
	assert.Equal(t, agents.DeliveryRewakeHook, claude.Delivery)
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

// selectingAgent resolves an on-disk descriptor that declares both selection
// blocks, plus a prompt delivery whose strategy is NOT restart_tui — the shape
// no shipped descriptor has, and the one that proves the selection block's own
// strategy is what forces a restart.
func selectingAgent(t *testing.T) agents.Agent {
	t.Helper()
	home := t.TempDir()
	writeDescriptor(t, home, "picker", `
id: picker
spawn:
  cmd: picker-cli
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: rewake_hook
    rewake:
      sentinel: "crowbar-delivered"
      summary: "Message from Crowbar chat"
      strip: '(?s)\A<system-reminder>\n{sentinel} ?(?P<message>.*)\n</system-reminder>\z'
      wake_status: 2
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low, high]
    opus: [max]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop:     { message: last }
`)
	a, err := agents.New().Get(context.Background(), home, "picker")
	require.NoError(t, err)
	return a
}

// TestAgent_SelectionCapabilitiesAreFactsAboutTheDescriptor pins the two shipped
// answers. claude declares both catalogues; codex declares neither, because its
// real catalogue is per-model and lives behind a command — a hand-written list
// would be wrong, so it degrades to no picker at all.
func TestAgent_SelectionCapabilitiesAreFactsAboutTheDescriptor(t *testing.T) {
	claude := get(t, "claude")
	assert.True(t, claude.Capabilities().ModelSelect)
	assert.True(t, claude.Capabilities().EffortSelect)
	assert.Equal(t, []string{"sonnet", "opus", "haiku"}, claude.Models())
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.Efforts(""))
	assert.Equal(t, claude.Efforts(""), claude.Efforts("opus"),
		"claude's levels do not vary by model, so every model takes the fallback")

	codex := get(t, "codex")
	assert.True(t, codex.Capabilities().ModelSelect)
	assert.True(t, codex.Capabilities().EffortSelect)
	assert.Equal(t, []string{
		"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini",
	}, codex.Models(), "codex's own priority order, as `codex debug models` reports it")
}

// TestAgent_CodexEffortsVaryByModel is the case the per-model catalogue shape
// exists for, and it is not hypothetical: measured 2026-08-17 against codex-cli
// 0.146.0, sol and terra reach ultra, luna stops at max, and the 5.4/5.5 family
// stops at xhigh.
//
// The last assertion is the important one. codex declares NO "*" fallback, so an
// id outside its catalogue must answer with NO levels — offering another model's
// levels would be a picker that produces spawn arguments the CLI rejects.
func TestAgent_CodexEffortsVaryByModel(t *testing.T) {
	codex := get(t, "codex")

	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		codex.Efforts("gpt-5.6-sol"))
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"},
		codex.Efforts("gpt-5.6-luna"))
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"},
		codex.Efforts("gpt-5.4-mini"))
	assert.Empty(t, codex.Efforts("gpt-9-imaginary"))
	assert.Empty(t, codex.Efforts(""), "no fallback key means the default model has no declared levels")
}

// TestAgent_CodexSelectionUsesItsOwnConfigChannel: codex has no --effort flag, so
// the level travels through its `-c key=value` config-override channel — the same
// shape every other codex config step uses.
func TestAgent_CodexSelectionUsesItsOwnConfigChannel(t *testing.T) {
	codex := get(t, "codex")
	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(),
		Model: "gpt-5.6-sol", Effort: "ultra",
	}

	steps := codex.SelectionSteps(agents.Selection{Model: "gpt-5.6-sol", Effort: "ultra"})
	plan, err := codex.SpawnPlan(ctx, nil, steps)

	require.NoError(t, err)
	modelAt := indexOf(plan.Argv, "--model")
	require.GreaterOrEqual(t, modelAt, 0)
	assert.Equal(t, "gpt-5.6-sol", plan.Argv[modelAt+1])
	assert.Contains(t, plan.Argv, `model_reasoning_effort="ultra"`)
}

// TestAgent_SelectionStepsCarryTheChoiceIntoTheArgv walks the whole path a
// selection takes: declared block → steps → rendered argv.
func TestAgent_SelectionStepsCarryTheChoiceIntoTheArgv(t *testing.T) {
	a := selectingAgent(t)
	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(),
		Model: "opus", Effort: "max",
	}

	steps := a.SelectionSteps(agents.Selection{Model: "opus", Effort: "max"})
	plan, err := a.SpawnPlan(ctx, nil, steps)

	require.NoError(t, err)
	assert.Equal(t, []string{"--model", "opus", "--effort", "max"}, plan.Argv)
}

// TestAgent_UnselectedSpawnIsArgvIdenticalToOneWithNoSelectionSupport is the
// inert-path guarantee, and it is the property the whole feature is allowed to
// rest on: a chat that has chosen nothing must launch the exact command line it
// launched before any of this existed.
//
// It is asserted against a REAL shipped descriptor (claude, which declares both
// blocks) rather than a stub, because a stub declaring nothing could not tell
// "contributes nothing" from "has nothing to contribute".
func TestAgent_UnselectedSpawnIsArgvIdenticalToOneWithNoSelectionSupport(t *testing.T) {
	a := get(t, "claude")
	// One tmp dir for both renders: it lands in the argv (--settings) and a fresh
	// one per call would differ for a reason that has nothing to do with selection.
	base := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), Segid: "SEG", Provider: "claude",
		ProjectID: "P", WorkspaceID: "W", CrowbarHook: "/bin/crowbar",
	}

	without, err := a.SpawnPlan(base, nil, nil)
	require.NoError(t, err)
	withEmpty, err := a.SpawnPlan(base, nil, a.SelectionSteps(agents.Selection{}))
	require.NoError(t, err)

	assert.Empty(t, a.SelectionSteps(agents.Selection{}))
	assert.Equal(t, without.Argv, withEmpty.Argv)
	for _, arg := range without.Argv {
		assert.NotContains(t, arg, "--model")
		assert.NotContains(t, arg, "--effort")
	}
}

// TestAgent_SelectionRestartIsAuthorisedByTheBlocksOwnStrategy is the forced
// restart, proved on a delivery path that would NOT otherwise restart: this
// descriptor declares rewake_hook, so nothing about delivering a prompt respawns
// the CLI — and a changed model still must.
func TestAgent_SelectionRestartIsAuthorisedByTheBlocksOwnStrategy(t *testing.T) {
	a := selectingAgent(t)
	require.NotEqual(t, agents.DeliveryRestartTUI, a.Capabilities().Delivery,
		"the fixture must not restart for delivery reasons, or this proves nothing")

	assert.False(t, a.SelectionRestart(
		agents.Selection{Model: "opus"}, agents.Selection{Model: "opus"}))
	assert.True(t, a.SelectionRestart(
		agents.Selection{}, agents.Selection{Model: "opus"}))
	assert.True(t, a.SelectionRestart(
		agents.Selection{Effort: "high"}, agents.Selection{}))
}

// TestAgent_SelectionIsAbsentWhereNothingIsDeclared keeps a provider with no
// catalogue out of every branch: nothing to offer, nothing to render, nothing to
// restart for.
func TestAgent_SelectionIsAbsentWhereNothingIsDeclared(t *testing.T) {
	a := stubAgent(t, "true")

	assert.Empty(t, a.Models())
	assert.Empty(t, a.Efforts(""))
	assert.Empty(t, a.SelectionSteps(agents.Selection{Model: "opus", Effort: "max"}))
	assert.False(t, a.SelectionRestart(agents.Selection{}, agents.Selection{Model: "opus"}))
}

// The SHIPPED descriptor's answer shapes, pinned against what was MEASURED
// against claude 2.1.234 on 2026-08-18.
//
// The hookSpecificOutput wrapper is the load-bearing part and the reason this is
// a test rather than a comment: a bare {"decision":{"behavior":…}} was measured
// failing claude's own hook-output validator — `Hook JSON output validation
// failed — (root): Invalid input` — after which the TUI dialog was drawn and the
// human's decision was thrown away. A descriptor edit that loses the wrapper
// would produce a channel that looks wired up and silently answers nothing.
func TestAgent_ClaudeAnswersItsPermissionInTheMeasuredWrappedShape(t *testing.T) {
	a := get(t, "claude")

	capability, ok := a.AnswerCapability(agents.HookPermission)
	require.True(t, ok, "claude declares an answer channel for its permission hook")
	assert.Equal(t, []string{"allow", "answer", "deny"}, capability.Keys)
	assert.Positive(t, capability.Wait)
	assert.Less(t, capability.Wait, 300*time.Second,
		"the daemon's budget must expire BEFORE the 300s timeout injected on the hook, "+
			"or the relay is killed mid-write instead of exiting under its own control")

	allow, err := a.RenderAnswer(agents.HookPermission, nil, agents.AnswerDecision{Key: "allow"})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest",`+
			`"decision":{"behavior":"allow"}}}`,
		string(allow))

	deny, err := a.RenderAnswer(agents.HookPermission, nil,
		agents.AnswerDecision{Key: "deny", Reason: "no"})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest",`+
			`"decision":{"behavior":"deny","message":"no"}}}`,
		string(deny))
}

// AskUserQuestion is answered THROUGH the permission hook — the one open question
// the answer channel had, settled by measurement — by handing the tool its own
// input back with the picks merged in.
func TestAgent_ClaudeAnswersAQuestionByEchoingTheToolInput(t *testing.T) {
	raw := []byte(`{"tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"A or B?"}]}}`)

	got, err := get(t, "claude").RenderAnswer(agents.HookPermission, raw,
		agents.AnswerDecision{Key: "answer", Answers: map[string]any{"A or B?": "A"}})

	require.NoError(t, err)
	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow",`+
			`"updatedInput":{"questions":[{"question":"A or B?"}],"answers":{"A or B?":"A"}}}}}`,
		string(got))
}

func TestAgent_ClaudeAnswersAnElicitationWithTheMCPVerbs(t *testing.T) {
	a := get(t, "claude")
	capability, ok := a.AnswerCapability(agents.HookElicitation)
	require.True(t, ok)
	assert.Equal(t, []string{"accept", "cancel", "decline"}, capability.Keys)

	got, err := a.RenderAnswer(agents.HookElicitation, nil,
		agents.AnswerDecision{Key: "accept", Content: []byte(`{"choice":"B"}`)})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"Elicitation","action":"accept","content":{"choice":"B"}}}`,
		string(got))
}

// A provider that declares no answer channel is completely unaffected: nothing it
// opens is answerable, so no relay of its is ever held open.
func TestAgent_CodexDeclaresNoAnswerChannel(t *testing.T) {
	a := get(t, "codex")

	_, ok := a.AnswerCapability(agents.HookPermission)
	assert.False(t, ok)

	_, err := a.RenderAnswer(agents.HookPermission, nil, agents.AnswerDecision{Key: "allow"})
	assert.ErrorIs(t, err, agents.ErrNotAnswerable)
}

// A decision claude has no template for is refused rather than approximated: its
// permission_suggestions ride a channel that was never measured, and a broader
// grant that silently narrowed to a plain allow would grant less than the user
// chose while reporting success.
func TestAgent_ClaudeRefusesASuggestionItCannotExpress(t *testing.T) {
	_, err := get(t, "claude").RenderAnswer(agents.HookPermission, nil,
		agents.AnswerDecision{Key: agents.ChoiceOptionSuggestion})

	assert.ErrorIs(t, err, agents.ErrUnsupportedDecision)
}

// Every hook Crowbar HOLDS OPEN must carry an explicit timeout in the injected
// settings: the default is the provider's, and a budget Crowbar does not own is
// one it cannot stay inside.
func TestAgent_ClaudeInjectsAnExplicitTimeoutOnEveryHookItHoldsOpen(t *testing.T) {
	tmp := t.TempDir()
	plan, err := get(t, "claude").SpawnPlan(agents.TemplateCtx{
		Tmp: tmp, Segid: "seg", CrowbarHook: "/bin/crowbar", Cwd: tmp,
	}, nil, nil)
	require.NoError(t, err)
	if plan.Cleanup != nil {
		t.Cleanup(plan.Cleanup)
	}

	settings, err := os.ReadFile(filepath.Join(tmp, "settings.json"))
	require.NoError(t, err, "claude's hooks are injected through a settings file")

	var decoded struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Timeout int `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(settings, &decoded))
	for _, event := range []string{"PermissionRequest", "Elicitation"} {
		matchers := decoded.Hooks[event]
		require.NotEmpty(t, matchers, event)
		require.NotEmpty(t, matchers[0].Hooks, event)
		assert.Positive(t, matchers[0].Hooks[0].Timeout,
			"%s is held open while a human decides and must declare its own budget", event)
	}
}

// --- Terminal prompts: the modals no hook reports ---

// TestMatchTerminalPrompt_ClaudeIdentifiesItsTrustDialog drives the SHIPPED
// descriptor against the trust screen captured from claude 2.1.207 and recorded in
// tests/integration/agent/barriers_test.go. It is the end-to-end proof that the
// needles in claude.yaml still match the screen they were taken from — the check
// that would fail first if a claude release repainted that dialog.
func TestMatchTerminalPrompt_ClaudeIdentifiesItsTrustDialog(t *testing.T) {
	screen := strings.Join([]string{
		"╭──────────────────────────────────────╮",
		"│ Do you trust the files in this folder?│",
		"│ ❯ 1. Yes, I trust this folder         │",
		"│   2. No, exit                         │",
		"│ Enter to confirm · Esc to cancel      │",
		"╰──────────────────────────────────────╯",
	}, "\n")

	prompt, ok := get(t, "claude").MatchTerminalPrompt(screen)

	require.True(t, ok)
	assert.Equal(t, agents.TerminalPromptTrust, prompt.Kind)
}

// TestMatchTerminalPrompt_CodexReportsAGenericBlock pins the deliberate asymmetry
// between the two shipped descriptors. Nothing on codex's dialog says anything
// about trust, so all it can truthfully report is THAT the CLI is blocked — and a
// client renders that as "waiting for input in the terminal" rather than guessing.
func TestMatchTerminalPrompt_CodexReportsAGenericBlock(t *testing.T) {
	screen := "› 1. Yes, continue\n  2. No, exit\n  Press enter to continue"

	prompt, ok := get(t, "codex").MatchTerminalPrompt(screen)

	require.True(t, ok)
	assert.Empty(t, prompt.Kind, "codex declares no kinded needle; naming one would be a guess")
}

// TestMatchTerminalPrompt_AnOrdinaryScreenIsNotABlock is the false-positive guard.
// A working agent's screen must never report a block, or the banner means nothing.
func TestMatchTerminalPrompt_AnOrdinaryScreenIsNotABlock(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		_, ok := get(t, id).MatchTerminalPrompt("> Ready.\n  shift+tab to cycle · ? for shortcuts")
		assert.False(t, ok, id)
	}
}

// TestCapabilities_TerminalPromptsIsDeclaredByBothShippedAgents backs the read a
// caller makes to skip a screen render entirely for a provider that could never
// match.
func TestCapabilities_TerminalPromptsIsDeclaredByBothShippedAgents(t *testing.T) {
	assert.True(t, get(t, "claude").Capabilities().TerminalPrompts)
	assert.True(t, get(t, "codex").Capabilities().TerminalPrompts)
}

// TestMatchTerminalPrompt_ProviderDeclaringNoneNeverMatches is the degradation
// story stated against a real override: a descriptor with no terminal_prompts
// block answers false on every screen, so its chats behave exactly as they did
// before this existed.
func TestMatchTerminalPrompt_ProviderDeclaringNoneNeverMatches(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "silent.yaml"), []byte(
		"id: silent\nspawn:\n  cmd: silent-cli\n  interactive_required: true\n"+
			"hooks:\n  format: json\n  events:\n"+
			"    session_start: { session_id: session_id }\n"+
			"    turn_stop: { message: last }\n"), 0o600))

	a, err := agents.New().Get(context.Background(), home, "silent")
	require.NoError(t, err)

	_, ok := a.MatchTerminalPrompt("❯ 1. Yes, I trust this folder\nEnter to confirm")
	assert.False(t, ok)
	assert.False(t, a.Capabilities().TerminalPrompts)
}
