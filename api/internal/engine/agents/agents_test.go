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

	assert.Equal(t, agents.DeliveryRestartTUI, codex.Delivery)
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
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
telemetry:
  probe:
    format: json
    command: ["-c", "cat testdata/probe_telemetry.json"]
    fields:
      context.capacity_tokens: cap
`)
	a, err := agents.New().Get(context.Background(), home, "polled")
	require.NoError(t, err)

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
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
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

func stubAgent(t *testing.T, cmd string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	writeDescriptor(t, home, "stub", `
id: stub
spawn:
  cmd: `+cmd+`
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
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
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
`)
	a, err := agents.New().Get(context.Background(), home, "picker")
	require.NoError(t, err)
	return a
}

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

func TestAgent_UnselectedSpawnIsArgvIdenticalToOneWithNoSelectionSupport(t *testing.T) {
	a := get(t, "claude")

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

func TestAgent_SelectionRestartIsAuthorisedByTheBlocksOwnStrategy(t *testing.T) {
	a := selectingAgent(t)
	require.NotEqual(t, agents.DeliveryRestartTUI, a.Capabilities().Delivery,
		"the fixture must not restart for delivery reasons, or this proves nothing")

	assert.False(t, a.SelectionRestart(
		agents.Selection{Model: "opus"}, agents.Selection{Model: "opus"},
	))
	assert.True(t, a.SelectionRestart(
		agents.Selection{}, agents.Selection{Model: "opus"},
	))
	assert.True(t, a.SelectionRestart(
		agents.Selection{Effort: "high"}, agents.Selection{},
	))
}

func TestAgent_SelectionIsAbsentWhereNothingIsDeclared(t *testing.T) {
	a := stubAgent(t, "true")

	assert.Empty(t, a.Models())
	assert.Empty(t, a.Efforts(""))
	assert.Empty(t, a.SelectionSteps(agents.Selection{Model: "opus", Effort: "max"}))
	assert.False(t, a.SelectionRestart(agents.Selection{}, agents.Selection{Model: "opus"}))
}

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

// codex's permission is now ANSWERABLE: the merged mixed-transport descriptor
// carries it over the api transport, which declares real allow/deny reply
// templates — unlike the old hooks-only observation this replaces (design spec
// §2.1, §2.3: "Permission requests become answerable from the chat").
func TestAgent_CodexDeclaresAnAnswerChannelForPermission(t *testing.T) {
	a := get(t, "codex")

	cap, ok := a.AnswerCapability(agents.HookPermission)
	require.True(t, ok)
	assert.True(t, cap.Accepts(agents.ChoiceOptionAllow))
	assert.True(t, cap.Accepts(agents.ChoiceOptionDeny))

	stdout, err := a.RenderAnswer(agents.HookPermission, nil, agents.AnswerDecision{Key: agents.ChoiceOptionAllow})
	require.NoError(t, err)
	assert.JSONEq(t, `{"decision":"approved"}`, string(stdout))
}

func TestAgent_ClaudeRefusesASuggestionItCannotExpress(t *testing.T) {
	_, err := get(t, "claude").RenderAnswer(agents.HookPermission, nil,
		agents.AnswerDecision{Key: agents.ChoiceOptionSuggestion})

	assert.ErrorIs(t, err, agents.ErrUnsupportedDecision)
}

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

func TestMatchTerminalPrompt_CodexReportsAGenericBlock(t *testing.T) {
	screen := "› 1. Yes, continue\n  2. No, exit\n  Press enter to continue"

	prompt, ok := get(t, "codex").MatchTerminalPrompt(screen)

	require.True(t, ok)
	assert.Empty(t, prompt.Kind, "codex declares no kinded needle; naming one would be a guess")
}

func TestMatchTerminalPrompt_AnOrdinaryScreenIsNotABlock(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		_, ok := get(t, id).MatchTerminalPrompt("> Ready.\n  shift+tab to cycle · ? for shortcuts")
		assert.False(t, ok, id)
	}
}

func TestCapabilities_TerminalPromptsIsDeclaredByBothShippedAgents(t *testing.T) {
	assert.True(t, get(t, "claude").Capabilities().TerminalPrompts)
	assert.True(t, get(t, "codex").Capabilities().TerminalPrompts)
}

func TestMatchTerminalPrompt_ProviderDeclaringNoneNeverMatches(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "silent.yaml"), []byte(
		"id: silent\nspawn:\n  cmd: silent-cli\n  interactive_required: true\n"+
			"hooks:\n  format: json\n  events:\n"+
			"    session_start: { session_id: session_id }\n"+
			"    turn_stop: { message: last }\n",
	), 0o600))

	a, err := agents.New().Get(context.Background(), home, "silent")
	require.NoError(t, err)

	_, ok := a.MatchTerminalPrompt("❯ 1. Yes, I trust this folder\nEnter to confirm")
	assert.False(t, ok)
	assert.False(t, a.Capabilities().TerminalPrompts)
}

const v3EventsBlock = `
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
`

func TestCapabilities_HasTerminalIsStructuralNotDeclared(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "hooks-transport", `
id: hooks-transport
spawn:
  cmd: hooks-cli
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: hooks
  hooks:
    format: json
`)
	hooksAgent, err := agents.New().Get(context.Background(), home, "hooks-transport")
	require.NoError(t, err)
	assert.True(t, hooksAgent.Capabilities().HasTerminal,
		"a hooks-transport provider's PTY IS its terminal")

	writeDescriptor(t, home, "api-no-attach", `
id: api-no-attach
spawn:
  cmd: api-cli
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [api-cli, serve]
    handshake: { call: initialize }
`)
	apiNoAttach, err := agents.New().Get(context.Background(), home, "api-no-attach")
	require.NoError(t, err)
	assert.False(t, apiNoAttach.Capabilities().HasTerminal,
		"served with no attach: there is no terminal, not a disabled one")

	writeDescriptor(t, home, "api-with-attach", `
id: api-with-attach
spawn:
  cmd: api-cli
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [api-cli, serve]
    attach: [api-cli, --remote]
    handshake: { call: initialize }
`)
	apiWithAttach, err := agents.New().Get(context.Background(), home, "api-with-attach")
	require.NoError(t, err)
	assert.True(t, apiWithAttach.Capabilities().HasTerminal)
}

func TestAgent_StartAPIConnRefusesAHooksTransportDescriptor(t *testing.T) {
	claude := get(t, "claude")
	_, err := claude.StartAPIConn(context.Background(), "/nonexistent.sock")
	assert.ErrorIs(t, err, agents.ErrAPITransportNotDeclared)
}

func TestAgent_APIServeAndAttachArgvAreAbsentForAHooksOnlyDescriptor(t *testing.T) {
	claude := get(t, "claude")
	_, ok := claude.APIServeArgv(agents.TemplateCtx{})
	assert.False(t, ok)
	_, ok = claude.APIAttachArgv(agents.TemplateCtx{})
	assert.False(t, ok)
}

func TestAgent_APIServeAndAttachArgvExpandTemplatesForAnAPITransportDescriptor(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-transport", `
id: api-transport
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    attach: [acme, --remote, "unix://{socket}"]
    handshake: { call: initialize }
`)
	a, err := agents.New().Get(context.Background(), home, "api-transport")
	require.NoError(t, err)

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{Socket: "/tmp/s.sock"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server", "--listen", "unix:///tmp/s.sock"}, serveArgv)

	attachArgv, ok := a.APIAttachArgv(agents.TemplateCtx{Socket: "/tmp/s.sock"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "--remote", "unix:///tmp/s.sock"}, attachArgv)
}

func TestAgent_TransportForResolvesPerEventOverridesAgainstTheRuntimeDefault(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "mixed-transport", `
id: mixed-transport
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: thread/started
    map:
      session_id: thread.id
  turn_stop:
    in: turn/completed
    map:
      message: turn.last
  session_end:
    transport: hooks
    in: SessionEnd
    map:
      session_id: session_id
runtime:
  transport: api
  hotswap: true
  api:
    protocol: jsonrpc2
    serve: [acme, app-server, --listen, "unix://{socket}"]
    handshake: { call: initialize }
  hooks:
    format: json
`)
	a, err := agents.New().Get(context.Background(), home, "mixed-transport")
	require.NoError(t, err)

	assert.Equal(t, "api", a.TransportFor("turn_stop"), "no override — the runtime default applies")
	assert.Equal(t, "hooks", a.TransportFor("session_end"), "declared transport: hooks overrides the api default")
}

func TestCapabilities_HotswapDefaultsFalse(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "undeclared", `
id: undeclared
spawn:
  cmd: x
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: hooks
  hooks:
    format: json
`)
	undeclared, err := agents.New().Get(context.Background(), home, "undeclared")
	require.NoError(t, err)
	assert.False(t, undeclared.Capabilities().Hotswap,
		"a descriptor that has not thought about hotswap gets the conservative answer")

	writeDescriptor(t, home, "declared", `
id: declared
spawn:
  cmd: x
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: hooks
  hotswap: true
  hooks:
    format: json
`)
	declared, err := agents.New().Get(context.Background(), home, "declared")
	require.NoError(t, err)
	assert.True(t, declared.Capabilities().Hotswap)
}
