package agents_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func get(t *testing.T, id string) agents.Agent {
	t.Helper()
	a, err := agents.New().Get(context.Background(), "", id)
	require.NoError(t, err)
	return a
}

const minimalOverride = `
id: probe
spawn:
  cmd: probe-cli
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
`

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
		ProjectID: "P", WorkspaceID: "W", CrowbarBin: "/bin/crowbar",
	}

	plan, err := a.SpawnPlan(ctx, os.Environ(), nil)

	require.NoError(t, err)
	assert.NotEmpty(t, plan.Executable)
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

				end := slices.Index(plan.Argv, "--")
				require.GreaterOrEqual(t, end, 0, "argv %v must end its options", plan.Argv)
				assert.Equal(t, "--print", plan.Argv[len(plan.Argv)-1],
					"a message that looks like a forbidden flag must survive as DATA")
			}
		})
	}
}

// TestAgent_ContextStepsDifferBetweenFreshAndResume covers claude: a fresh spawn
// rides silent config (--append-system-prompt), a resume rides a real positional
// user turn instead — --append-system-prompt on an already-native-resumed CLI is
// background config a model carrying its own restored history is free to
// deprioritize (confirmed live), which a positional turn is not.
func TestAgent_ContextStepsDifferBetweenFreshAndResume(t *testing.T) {
	claude := get(t, "claude")

	fresh := claude.ContextSteps(false)
	resumed := claude.ContextSteps(true)

	require.NotEmpty(t, fresh)
	require.NotEmpty(t, resumed)

	assert.Contains(t, fresh[0].Args, "arg")
	assert.Contains(t, resumed[0].Args, "positional")
}

// TestAgent_Codex_ResumeContextRidesThePositionalPrompt: a chat-surface resume
// carries the gap over the api connection, but the terminal surface's
// `codex resume <id> [PROMPT]` has only its positional prompt to carry it.
func TestAgent_Codex_ResumeContextRidesThePositionalPrompt(t *testing.T) {
	codex := get(t, "codex")

	assert.NotEmpty(t, codex.ContextSteps(false), "a fresh codex spawn still carries {context} as developer_instructions")
	resumed := codex.ContextSteps(true)
	require.Len(t, resumed, 1)
	assert.Equal(t, "{context}", resumed[0].Args["positional"])
}

func TestAgent_ContextStepsAreADefensiveCopy(t *testing.T) {
	a := get(t, "claude")

	first := a.ContextSteps(false)
	require.NotEmpty(t, first)
	first[0].Args["arg"] = "MUTATED"

	assert.NotEqual(t, "MUTATED", a.ContextSteps(false)[0].Args["arg"])
}

func TestAgent_ParseHookMapsAConversationTurn(t *testing.T) {
	a := get(t, "claude")

	ev, err := a.ParseHook(agents.HookTurnStop,
		[]byte(`{"session_id":"s1","last_assistant_message":"done"}`),
		agents.ChannelHooks)

	require.NoError(t, err)
	assert.Equal(t, "s1", ev.SessionID)
	assert.Equal(t, "done", ev.Message)
}

// subagent_pre stays hooks-only (no per-event transport override AND no
// api-transport equivalent at all), so an api-transport delivery can never
// reach it — the guard is unconditional here regardless of how it treats
// dual-shape events.
func TestAgent_ParseHookRefusesAnotherConversationsPayload(t *testing.T) {
	a := get(t, "codex")

	_, err := a.ParseHook(agents.HookSubagentPre,
		[]byte(`{"session_id":"s1","agent_id":"a1","agent_type":"t1","transcript_path":null}`),
		agents.ChannelHooks)

	assert.ErrorIs(t, err, agents.ErrForeignConversation)
}

// TestRegression_CodexMemoryConsolidationSessionDoesNotStealTheChat pins the
// exact live capture from the original chat-theft bug against the REAL,
// shipped codex.yaml (not a synthetic stand-in), through the same ParseHook
// entry point production uses.
//
// user_prompt has no per-event transport override, so it inherits the
// runtime's api default — but codex's spawn config still ALSO fires it
// hooks-shaped for its internal memory-consolidation session, which is
// exactly what a transport-wide skip (rather than a per-field presence
// check) let through: the event is classified "api", so the guard was
// skipped outright even though THIS delivery is hooks-shaped and foreign.
func TestRegression_CodexMemoryConsolidationSessionDoesNotStealTheChat(t *testing.T) {
	a := get(t, "codex")

	_, err := a.ParseHook(agents.HookUserPrompt,
		[]byte(`{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","turn_id":"019fafaf-4f54",`+
			`"transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"UserPromptSubmit",`+
			`"model":"gpt-5.6-terra","permission_mode":"bypassPermissions",`+
			`"prompt":"MEMORY-WRITING-AGENT-PHASE-2-CONSOLIDATION"}`),
		agents.ChannelHooks)

	assert.ErrorIs(t, err, agents.ErrForeignConversation,
		"a hooks-shaped delivery of a dual-shape event must still be checked, even though "+
			"user_prompt's declared transport is api")
}

// TestRegression_EveryDualShapeCodexEventRejectsAForeignHooksPayload sweeps
// every codex.yaml event sharing session_start/user_prompt/turn_stop's own
// hazard: no per-event transport override (so it inherits runtime.transport:
// api) AND a config_injection hooks.* entry that fires it, unconditionally,
// off the disconnected companion PTY (grep -n '||' codex.yaml plus
// config_injection's hooks.* pass_args names exactly this set).
// ownsConversation (hooks.go) is keyed only on RequiredPayloadFields, never on
// the canonical event name, so the fix that closed the memory-consolidation
// chat-theft bug for user_prompt must reject a foreign hooks-shaped delivery
// of every one of these the same way — proven here against the REAL,
// shipped codex.yaml rather than asserted from reading the mechanism.
func TestRegression_EveryDualShapeCodexEventRejectsAForeignHooksPayload(t *testing.T) {
	a := get(t, "codex")
	for _, event := range []string{
		agents.HookSessionStart, agents.HookUserPrompt, agents.HookTurnStop,
		agents.HookToolPre, agents.HookToolPost,
		agents.HookPermission, agents.HookCompactPre, agents.HookCompactPost,
	} {
		t.Run(event, func(t *testing.T) {
			require.Equal(t, "api", a.TransportFor(event),
				"precondition: this event must actually inherit the api default for the "+
					"sweep to mean anything")

			_, err := a.ParseHook(event, []byte(`{"transcript_path":null}`), agents.ChannelHooks)

			assert.ErrorIs(t, err, agents.ErrForeignConversation)
		})
	}
}

func TestAgent_ParseHookReportsAnUndeclaredEvent(t *testing.T) {
	_, err := get(t, "codex").ParseHook(agents.HookNotification,
		[]byte(`{"transcript_path":"/x","message":"hi"}`), agents.ChannelHooks)

	assert.ErrorIs(t, err, agents.ErrHookUndeclared)
}

// TestRegression_ParseHookReadsAChannelScopedEventsOwnBlock is the chat-theft
// class proven through the SAME public entry point production uses
// (Agent.ParseHook — ingest.go's own call), not just the internal translate/
// inbound package underneath it: the SAME canonical event, the SAME
// descriptor, two payloads shaped for two DIFFERENT channels, each read
// through its own channel's block. See translate/inbound/hooks.go's doc
// comment on Parse, and docs/plans/2026-09-22-descriptor-channel-split.md
// 1.1, for the live bug (transport-keyed rather than channel-keyed
// resolution) this design replaces.
func TestRegression_ParseHookReadsAChannelScopedEventsOwnBlock(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "channel-split", `
id: channel-split
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: session_start
    map: { session_id: session_id }
  turn_stop:
    in: turn_stop
    map: { message: last }
  tool_pre:
    required: [session_id, tool_id, tool_name]
    api:
      in: item/started
      when: { item.type: { any_of: [commandExecution, fileChange] } }
      map: { session_id: threadId, tool_id: item.id, tool_name: item.type }
    hooks:
      in: PreToolUse
      map: { session_id: session_id, tool_id: tool_use_id, tool_name: tool_name }
runtime:
  transport: hooks
  hooks:
    format: json
`)
	a, err := agents.New().Get(context.Background(), home, "channel-split")
	require.NoError(t, err)

	apiEv, err := a.ParseHook(agents.HookToolPre,
		[]byte(`{"threadId":"api-session","item":{"type":"commandExecution","id":"api-tool"}}`),
		agents.ChannelAPI)
	require.NoError(t, err)
	assert.Equal(t, "api-session", apiEv.SessionID)
	require.NotNil(t, apiEv.Tool)
	assert.Equal(t, "api-tool", apiEv.Tool.ID)

	hooksEv, err := a.ParseHook(agents.HookToolPre,
		[]byte(`{"session_id":"hooks-session","tool_use_id":"hooks-tool","tool_name":"Bash"}`),
		agents.ChannelHooks)
	require.NoError(t, err)
	assert.Equal(t, "hooks-session", hooksEv.SessionID)
	require.NotNil(t, hooksEv.Tool)
	assert.Equal(t, "hooks-tool", hooksEv.Tool.ID)

	// The isolation half: the api-shaped payload read through the HOOKS
	// channel must resolve nothing — never fall back to the api block's
	// threadId/item.id, which is exactly the cross-shape leak the old
	// transport-keyed resolution allowed. tool_pre declares session_id
	// required:, so isolation now surfaces as a RequiredFieldError rather
	// than a hollow success (design spec 2.3) — a stronger proof than an
	// empty field: the mismatch is REJECTED, not merely unfilled.
	_, err = a.ParseHook(agents.HookToolPre,
		[]byte(`{"threadId":"api-session","item":{"type":"commandExecution","id":"api-tool"}}`),
		agents.ChannelHooks)
	assert.ErrorIs(t, err, agents.ErrRequiredFieldMissing)
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

// This test used to assert codex declares NO telemetry channel. It did declare
// one — events.telemetry, mapping thread/tokenUsage/updated — but only the v2
// top-level telemetry.callback block was ever read, so every codex report came
// back ErrUnsupported, t.telemetry.Set was never called, and the context gauge
// (which renders nothing without a usedPercent) has never appeared on a codex chat.
//
// The genuinely-undeclared case is covered where it belongs, on a descriptor that
// declares telemetry neither way:
// translate/telemetry.TestParseCallback_UnsupportedWhenNeitherFormIsDeclared.
func TestRegression_CodexTelemetryReachesTheContextGauge(t *testing.T) {
	// Shape copied from the LIVE capture in
	// internal/protocol/testdata/fixtures/codex/thread_tokenUsage_updated.json —
	// modelContextWindow sits inside tokenUsage, not beside it.
	raw := []byte(`{"threadId":"t1","turnId":"tn1","tokenUsage":{
	  "total":{"totalTokens":16924,"inputTokens":16907,"outputTokens":17},
	  "last":{"totalTokens":16924,"inputTokens":16907,"outputTokens":17},
	  "modelContextWindow":258400}}`)

	got, err := get(t, "codex").ParseTelemetry(raw, time.Now())

	require.NoError(t, err)
	require.NotNil(t, got.Context, "no context usage means the gauge renders nothing")
	require.NotNil(t, got.Context.UsedTokens)
	assert.Equal(t, 16924, *got.Context.UsedTokens)
	require.NotNil(t, got.Context.CapacityTokens)
	assert.Equal(t, 258400, *got.Context.CapacityTokens)
	// The gauge renders nothing at all without a percentage; it is derived here.
	require.NotNil(t, got.Context.UsedPercent)
	assert.InDelta(t, 6.55, *got.Context.UsedPercent, 0.1)
}

// TestRegression_CodexContextPercentUsesLastTurnNotSessionTotal: tokenUsage.total
// is a lifetime counter that only grows turn over turn (codex's own TUI draws the
// identical distinction against the same wire shape — codex-rs/tui/src/token_usage.rs,
// tokens_in_context_window's doc comment). Mapping it into context.used_tokens divided
// a number that keeps climbing forever by the fixed context window, so a chat well
// past its first couple of turns rendered percentages over 1000% — observed live as
// "2519% of context used" on an ordinary long-running codex chat, nothing to do with
// a provider switch. tokenUsage.last — the current turn's own context size — is what
// must drive the gauge instead.
func TestRegression_CodexContextPercentUsesLastTurnNotSessionTotal(t *testing.T) {
	raw := []byte(`{"threadId":"t1","turnId":"tn9","tokenUsage":{
	  "total":{"totalTokens":6512000,"inputTokens":6500000,"outputTokens":12000},
	  "last":{"totalTokens":92800,"inputTokens":92000,"outputTokens":800},
	  "modelContextWindow":258400}}`)

	got, err := get(t, "codex").ParseTelemetry(raw, time.Now())

	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.NotNil(t, got.Context.UsedTokens)
	assert.Equal(t, 92800, *got.Context.UsedTokens, "the gauge must track the current turn's context size, not the session-wide lifetime total")
	require.NotNil(t, got.Context.UsedPercent)
	assert.InDelta(t, 35.9, *got.Context.UsedPercent, 0.1)
	assert.LessOrEqual(t, *got.Context.UsedPercent, 100.0, "a context gauge can never legitimately exceed 100%")
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

func TestShippedAgents_RenderParseableMCPRegistration(t *testing.T) {
	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), Segid: "SEG", RunnerToken: "TOK",
		ProjectID: "P", RepoID: "R", WorkspaceID: "W", CrowbarBin: "/bin/crowbar", DaemonHost: "tcp://127.0.0.1:3737",
	}

	claude, err := get(t, "claude").SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)
	cfgIdx := slices.Index(claude.Argv, "--mcp-config")
	require.GreaterOrEqual(t, cfgIdx, 0)
	require.Less(t, cfgIdx+2, len(claude.Argv), "the JSON must be followed by another FLAG")
	assert.True(t, strings.HasPrefix(claude.Argv[cfgIdx+2], "-"),
		"a bare positional after a variadic --mcp-config is swallowed as another config")
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(claude.Argv[cfgIdx+1]), &parsed))
	assert.Contains(t, claude.Argv[cfgIdx+1], `"--host","tcp://127.0.0.1:3737"`, "the relay dials the daemon's address")

	codex, err := get(t, "codex").SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(codex.Argv, " "), `"--host","tcp://127.0.0.1:3737"`, "the relay dials the daemon's address")
}

func hookCtx(tmp, provider, repo string) agents.TemplateCtx {
	return agents.TemplateCtx{
		Tmp: tmp, Cwd: filepath.Join(tmp, "cwd"), Segid: "SEG", RunnerToken: "TOK", Provider: provider,
		ProjectID: "P", RepoID: repo, WorkspaceID: "W", CrowbarBin: "/bin/crowbar",
		HookBase: "http://127.0.0.1:4242",
	}
}

// A project home has no repo id: its hooks must take the home route, never a
// dangling /repos//.
func TestShippedAgents_RenderHookURLsThatSurviveAnEmptyRepoID(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		for repo, scope := range map[string]string{"": "/v0/projects/P/home", "R": "/v0/projects/P/repos/R"} {
			t.Run(id+"/repo="+repo, func(t *testing.T) {
				tmp := t.TempDir()
				plan, err := get(t, id).SpawnPlan(hookCtx(tmp, id, repo), nil, nil)
				require.NoError(t, err)

				rendered := strings.Join(plan.Argv, " ") + " " + readAll(t, tmp)
				assert.Contains(t, rendered, "http://127.0.0.1:4242"+scope+"/chats/runners/SEG/hooks/"+id+"/tool_pre")
				assert.NotContains(t, rendered, "/repos//")
				assert.NotContains(t, rendered, "crowbar hook", "no relay process is launched per event")
			})
		}
	}
}

// Claude posts its hooks itself, authenticated by the runner token. Only the status
// line and SessionStart, which claude can only run as commands, go through curl.
func TestShippedClaude_HooksAreNativeHTTPHooksCarryingTheRunnerToken(t *testing.T) {
	tmp := t.TempDir()
	_, err := get(t, "claude").SpawnPlan(hookCtx(tmp, "claude", "R"), nil, nil)
	require.NoError(t, err)

	var settings struct {
		Hooks      map[string][]struct{ Hooks []map[string]any } `json:"hooks"`
		StatusLine map[string]any                                `json:"statusLine"`
	}
	require.NoError(t, json.Unmarshal([]byte(readFile(t, filepath.Join(tmp, "settings.json"))), &settings))
	require.NotEmpty(t, settings.Hooks)
	for event, groups := range settings.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if event == "SessionStart" {
					assert.Equal(t, "command", hook["type"], "claude never delivers SessionStart to an http hook")
					assert.Contains(t, hook["command"], "/hooks/claude/session_start")
					assert.NotContains(t, hook["command"], "TOK", "the token must not appear in argv")
					continue
				}
				assert.Equal(t, "http", hook["type"], event)
				assert.Contains(t, hook["url"], "/chats/runners/SEG/hooks/claude/", event)
				assert.Equal(t, map[string]any{"X-Crowbar-Runner-Token": "TOK"}, hook["headers"], event)
			}
		}
	}
	assert.Equal(t, "command", settings.StatusLine["type"], "claude's status line can only run a command")
	command, _ := settings.StatusLine["command"].(string)
	assert.True(t, strings.HasPrefix(command, "curl "), command)
	assert.Contains(t, command, "/hooks/claude/telemetry")
	assert.NotContains(t, command, "TOK", "the token must not appear in argv")
}

// Codex has no HTTP hooks; its command hooks post through curl instead of exec'ing a
// 50 MB relay, with the token in a private file rather than argv.
func TestShippedCodex_HooksPostThroughCurlReadingTheTokenFromAFile(t *testing.T) {
	tmp := t.TempDir()
	plan, err := get(t, "codex").SpawnPlan(hookCtx(tmp, "codex", "R"), nil, nil)
	require.NoError(t, err)

	argv := strings.Join(plan.Argv, " ")
	assert.Contains(t, argv, `hooks.PreToolUse=[{hooks=[{type="command",command="curl --fail --silent --show-error --config `)
	assert.Contains(t, argv, "/chats/runners/SEG/hooks/codex/tool_pre")
	for _, arg := range plan.Argv {
		if strings.HasPrefix(arg, "hooks.") {
			assert.NotContains(t, arg, "TOK", "a hook's command line must not carry the token")
		}
	}

	cfg := filepath.Join(tmp, "hook.curl")
	assert.Contains(t, readFile(t, cfg), `header = "X-Crowbar-Runner-Token: TOK"`)
	info, err := os.Stat(cfg)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o077, "the credential file is for the owner only")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
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

func TestAgent_PromptSteerCarriesTheMessageAsAJSONStringAndSkipsCommands(t *testing.T) {
	stdout, ok := get(t, "claude").PromptSteer("say \"hi\"\nthen {message_json}")

	require.True(t, ok)
	var reply struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &reply), "the reply must stay valid JSON whatever the user typed")
	assert.Equal(t, "block", reply.Decision)
	assert.Contains(t, reply.Reason, "say \"hi\"\nthen {message_json}")

	for _, command := range []string{"/compact", "!ls"} {
		_, ok := get(t, "claude").PromptSteer(command)
		assert.False(t, ok, command)
	}
	_, ok = get(t, "codex").PromptSteer("hello")
	assert.False(t, ok, "a provider that declares no steering is never steered")
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
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
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
	assert.Equal(t, []string{"fable", "opus", "sonnet", "haiku", "opusplan"}, claude.Models(),
		"resolved synchronously from the embedded model-manifest.json bundle")
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.Efforts(""),
		"the '' key is the union of every model's own levels — see modeldiscovery.effortsOf")
	assert.Equal(t, claude.Efforts(""), claude.Efforts("opus"),
		"the bundled manifest currently states the same levels for every model")
	assert.Empty(t, claude.DefaultModel(),
		"claude's manifest states no default (account-dependent) — never inferred, never Models[0]")

	codex := get(t, "codex")
	assert.True(t, codex.Capabilities().ModelSelect)
	assert.True(t, codex.Capabilities().EffortSelect)
}

// TestAgent_CodexModelsAreDiscoveredNotDeclared pins codex.yaml's own switch
// to model.discover: with no `codex` binary in this test environment (and
// homeDir "" — get's own helper — failing Probe's own cwd check outright),
// nothing ever resolves, so Models/Efforts read as "not yet known" (empty),
// never the stale hand-maintained list this replaced. The actual
// filter/order/per-model-effort mapping is modeldiscovery's own table-driven
// tests against a real trimmed capture; the cache-to-Agent wiring itself is
// TestAgent_ModelsAndEffortsReadTheDiscoveryCacheWhenDeclared below.
func TestAgent_CodexModelsAreDiscoveredNotDeclared(t *testing.T) {
	codex := get(t, "codex")

	assert.Empty(t, codex.Models(), "no probe has resolved, so nothing is known yet")
	assert.Empty(t, codex.Efforts("gpt-5.6-sol"))
	assert.Empty(t, codex.Efforts(""))
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
	modelAt := slices.Index(plan.Argv, "--model")
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
		ProjectID: "P", WorkspaceID: "W", CrowbarBin: "/bin/crowbar",
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
	assert.Equal(t, []string{"allow", "answer", "deny", "suggestion"}, capability.Keys)
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
	assert.JSONEq(t, `{"decision":"accept"}`, string(stdout))
}

// Picking claude's own "always" grant allows the call and hands the grant back as
// updatedPermissions, the shape its PermissionRequest hook applies.
func TestAgent_ClaudeGrantsTheSuggestionPicked(t *testing.T) {
	raw, err := os.ReadFile("internal/protocol/testdata/fixtures/claude/PermissionRequest.Edit.json")
	require.NoError(t, err)

	out, err := get(t, "claude").RenderAnswer(agents.HookPermission, raw,
		agents.AnswerDecision{Key: agents.ChoiceOptionSuggestion, Picked: []string{"suggestion-0"}})

	require.NoError(t, err)
	assert.JSONEq(t, `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow",`+
		`"updatedPermissions":[{"type":"setMode","mode":"acceptEdits","destination":"session"}]}}}`, string(out))
}

func TestAgent_ClaudeInjectsAnExplicitTimeoutOnEveryHookItHoldsOpen(t *testing.T) {
	tmp := t.TempDir()
	plan, err := get(t, "claude").SpawnPlan(agents.TemplateCtx{
		Tmp: tmp, Segid: "seg", CrowbarBin: "/bin/crowbar", Cwd: tmp,
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

// A daemon started inside a Claude Code session inherits that session's
// identity; passed on, every chat's CLI would adopt the one session id
// (found by the live conformance run).
func TestAgent_ClaudeNeverInheritsASessionIdentity(t *testing.T) {
	tmp := t.TempDir()
	inherited := []string{
		"PATH=/bin", "CLAUDE_CODE_SESSION_ID=parent", "CLAUDE_CODE_REMOTE_SESSION_ID=parent",
		"CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_PID=1", "CLAUDECODE=1",
		// The parent's peer channel and account identity: a child reached the
		// parent's messaging socket and wrote its user's email into prompts.
		"CLAUDE_CODE_MESSAGING_SOCKET=/tmp/p.sock", "CLAUDE_CODE_MESSAGING_TOKEN=t",
		"CLAUDE_CODE_USER_EMAIL=a@b", "CLAUDE_CODE_ACCOUNT_UUID=u", "CLAUDE_CODE_ORGANIZATION_UUID=o",
		"CLAUDE_CODE_SESSION_ATTENDED=1", "CLAUDE_SESSION_INGRESS_TOKEN_FILE=/f",
	}
	plan, err := get(t, "claude").SpawnPlan(agents.TemplateCtx{
		Tmp: tmp, Segid: "seg", CrowbarBin: "/bin/crowbar", Cwd: tmp,
	}, inherited, nil)
	require.NoError(t, err)
	if plan.Cleanup != nil {
		t.Cleanup(plan.Cleanup)
	}
	assert.Equal(t, []string{"PATH=/bin"}, plan.Env)
}

// A claude never set up on this machine paints its onboarding before any
// hook: both screens read as blocking, never as a healthy empty pane.
func TestMatchTerminalPrompt_ClaudeFirstRunOnboardingBlocks(t *testing.T) {
	for _, screen := range []string{
		"Welcome to Claude Code v2.1.281\n Let's get started.\n Choose the text style that looks best with your terminal\n ❯ 2. Dark mode ✔",
		"Claude Code can be used with your Claude subscription\n Select login method:\n ❯ Claude account with subscription",
	} {
		_, ok := get(t, "claude").MatchTerminalPrompt(screen)
		assert.True(t, ok, "not recognised as blocking: %q", screen)
	}
}

// A command that opens a dialog (/help) holds the CLI until the person closes it
// in the terminal; its screen (captured on 2.1.319) reads as blocking.
func TestMatchTerminalPrompt_ClaudeCommandDialogBlocks(t *testing.T) {
	screen := " Help General Commands Custom commands\n Shortcuts ! for shell mode\n" +
		" For more help: https://code.claude.com/docs/en/overview\n Esc to cancel"

	prompt, ok := get(t, "claude").MatchTerminalPrompt(screen)

	require.True(t, ok)
	assert.Empty(t, prompt.Kind)
}

// The trust dialog as the real CLI paints it (2.1.319, through our emulator) preselects
// "No, exit": accepting must move the cursor onto the trust option before confirming.
func TestMatchTerminalPrompt_ClaudeAimsItsTrustAnswerAtTheTrustOption(t *testing.T) {
	screen, err := os.ReadFile("testdata/claude-trust-2.1.319.screen")
	require.NoError(t, err)

	prompt, ok := get(t, "claude").MatchTerminalPrompt(string(screen))

	require.True(t, ok)
	assert.Equal(t, agents.TerminalPromptTrust, prompt.Kind)
	require.Len(t, prompt.Answers, 1)
	assert.Equal(t, "\x1b[B\r", prompt.Answers[0].Keys)
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

// apiRuntime opens an api-transport runtime; a fixture adds its own serve.
const apiRuntime = `runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: stdio
    format: ndjson
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
`+v3EventsBlock+apiRuntime+`
    serve: [api-cli, serve]
    handshake: { call: initialize, notify: initialized }
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
`+v3EventsBlock+apiRuntime+`
    serve: [api-cli, serve]
    attach: [api-cli, --remote]
    handshake: { call: initialize, notify: initialized }
`)
	apiWithAttach, err := agents.New().Get(context.Background(), home, "api-with-attach")
	require.NoError(t, err)
	assert.True(t, apiWithAttach.Capabilities().HasTerminal)
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
    delivery: stdio
    format: ndjson
    serve: [acme, app-server]
    handshake: { call: initialize, notify: initialized }
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

// TestCapabilities_TerminalStartHereReadsTheSurfacesBlock proves design spec
// 2.5's `surfaces.terminal.start_here` reaches Capabilities — absent by
// default (no surfaces: block at all, or a terminal surface that omits it),
// true only when declared, same conservative direction as every other
// capability key.
func TestCapabilities_TerminalStartHereReadsTheSurfacesBlock(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "no-surfaces", `
id: no-surfaces
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
	noSurfaces, err := agents.New().Get(context.Background(), home, "no-surfaces")
	require.NoError(t, err)
	assert.False(t, noSurfaces.Capabilities().TerminalStartHere,
		"a descriptor with no surfaces: block declares no launch surface at all")

	writeDescriptor(t, home, "start-here", `
id: start-here
spawn:
  cmd: x
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: hooks
  hotswap: true
  hooks:
    format: json
surfaces:
  chat: { channel: hooks, start_here: true }
  terminal: { channel: hooks, start_here: true }
`)
	startHere, err := agents.New().Get(context.Background(), home, "start-here")
	require.NoError(t, err)
	assert.True(t, startHere.Capabilities().TerminalStartHere)

	writeDescriptor(t, home, "not-start-here", `
id: not-start-here
spawn:
  cmd: x
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: hooks
  hotswap: true
  hooks:
    format: json
surfaces:
  chat: { channel: hooks, start_here: true }
  terminal: { channel: hooks }
`)
	notStartHere, err := agents.New().Get(context.Background(), home, "not-start-here")
	require.NoError(t, err)
	assert.False(t, notStartHere.Capabilities().TerminalStartHere,
		"terminal declared but start_here omitted must not offer the CLI-start affordance")
}

// TestSurfaceNames_MatchTheDescriptorVocabulary pins the two spellings of the
// same two surfaces together: the descriptor schema's (this package, from
// spec) and persistence's (domain.Chat.Surface). The string crosses both
// layers and goes out on the wire, and the layers deliberately do not import
// each other — so this is the only thing standing between them and a silent
// divergence that would make every gate read the wrong surface.
func TestSurfaceNames_MatchTheDescriptorVocabulary(t *testing.T) {
	assert.Equal(t, domain.SurfaceChat, agents.SurfaceChat)
	assert.Equal(t, domain.SurfaceTerminal, agents.SurfaceTerminal)
	assert.True(t, domain.KnownSurface(agents.SurfaceChat))
	assert.True(t, domain.KnownSurface(agents.SurfaceTerminal))
}
