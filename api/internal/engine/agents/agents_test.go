package agents_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestRegression_GetCachesAResolvedDescriptorUntilItsOverrideFileChanges is
// the fix for a live-reported bug: Codex's streaming view visibly stalled
// mid-reply, self-correcting only once the turn ended, while Claude streamed
// smoothly. Get sits on the ingest hot path (once per hook, including once
// per streamed delta), and protocol.Resolve is deliberately uncached — a full
// disk read, YAML parse and rule-validate on every call, over a millisecond
// for codex's own descriptor, measured. Codex's much higher per-second event
// rate over its api-transport connection paid that cost often enough to fall
// behind the incoming stream; Claude's hooks-paced rate never did. This
// proves the cache holds an override's resolved descriptor stable across
// repeated Get calls — corrupting the override on disk WITHOUT changing its
// mtime must not be noticed, because noticing would mean Resolve ran again.
func TestRegression_GetCachesAResolvedDescriptorUntilItsOverrideFileChanges(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "probe.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalOverride), 0o600))

	svc := agents.New()
	ctx := context.Background()
	first, err := svc.Get(ctx, home, "probe")
	require.NoError(t, err)
	assert.Equal(t, "probe", first.ID())

	info, err := os.Stat(path)
	require.NoError(t, err)
	stableModTime := info.ModTime()

	// Corrupted, but pinned back to the EXACT mtime Get already saw — no
	// sleep, no timing dependency: the cache is asked to distinguish "this
	// file's content is different" (which it cannot, and must not try to)
	// from "this file's mtime is different" (which it can, cheaply).
	require.NoError(t, os.WriteFile(path, []byte("id: [unclosed"), 0o600))
	require.NoError(t, os.Chtimes(path, stableModTime, stableModTime))

	_, err = svc.Get(ctx, home, "probe")
	require.NoError(t, err, "THE FIX: an unchanged mtime serves the cached descriptor, "+
		"never re-reading a file that Get has no reason to believe changed")

	// Now genuinely invalidate it: a real mtime change must still be caught,
	// or this would not be a cache with correct invalidation — it would just
	// be permanently stale, and a developer's edited override would never
	// take effect without a daemon restart.
	changedModTime := stableModTime.Add(time.Second)
	require.NoError(t, os.Chtimes(path, changedModTime, changedModTime))

	_, err = svc.Get(ctx, home, "probe")
	assert.Error(t, err, "a real mtime change must invalidate the cache and surface the now-broken override")
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

// TestAgent_Codex_ResumeContextStepsIsEmpty: codex's resume-context delivery does
// NOT ride ContextSteps at all — a resumed codex thread is entirely owned by the
// api connection (apiOwnsResume, chat/internal/runner/prompts.go), which delivers
// the identical {context} document over its own live socket (InjectAt "context",
// apiconn.go) instead of any CLI argv. codex.yaml declaring no
// resume_context_inject is that fact made explicit, not an oversight — asserted
// here so a reintroduced stanza (unreachable dead weight; spawn.go's own
// apiOwnsResume gate already withholds ContextSteps(true) from codex's redundant
// PTY) fails loudly instead of silently.
func TestAgent_Codex_ResumeContextStepsIsEmpty(t *testing.T) {
	codex := get(t, "codex")

	assert.NotEmpty(t, codex.ContextSteps(false), "a fresh codex spawn still carries {context} as developer_instructions")
	assert.Empty(t, codex.ContextSteps(true), "a resumed codex thread's context now rides the api connection, not CLI argv")
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
		[]byte(`{"session_id":"s1","last_assistant_message":"done","background_tasks":[1]}`),
		agents.ChannelHooks)

	require.NoError(t, err)
	assert.Equal(t, "s1", ev.SessionID)
	assert.Equal(t, "done", ev.Message)
	assert.Equal(t, 1, ev.AsyncWork)
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

func TestDecide_IsReExportedAsAPureFunction(t *testing.T) {
	assert.Equal(t, agents.MoveNoop, agents.Decide("s1", "s1", "", false, false).Kind)
	assert.Equal(t, agents.MoveBind, agents.Decide("", "s1", "", false, false).Kind)
	assert.Equal(t, agents.MoveToNew, agents.Decide("s1", "s2", "", false, false).Kind)
	assert.Equal(t, agents.MoveBind, agents.Decide("s1", "s2", "", false, true).Kind)

	known := agents.Decide("s1", "s2", "chat-9", true, false)
	assert.Equal(t, agents.MoveToKnown, known.Kind)
	assert.Equal(t, "chat-9", known.ChatID)
}

func TestInjectionRegistry_RecognisesAnEchoOncePerRunner(t *testing.T) {
	e := agents.New()
	e.RecordInjection("runner-1", "handoff blob")

	remainder, found := e.ConsumeInjectedPrefix("runner-1", "handoff blob")
	assert.True(t, found)
	assert.Empty(t, remainder)
	_, found = e.ConsumeInjectedPrefix("runner-1", "handoff blob")
	assert.False(t, found)

	e.RecordInjection("runner-2", "other")
	e.ForgetRunner("runner-2")
	_, found = e.ConsumeInjectedPrefix("runner-2", "other")
	assert.False(t, found)
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

// manifestDescriptorYAML is a synthetic model.manifest: descriptor pointing
// at url — the shared embedded model-manifest.json has no "synthetic"
// provider key, so this id's embedded half always resolves empty, letting a
// test tell "the network was actually consulted" apart from "the embedded
// fallback still applies".
func manifestDescriptorYAML(url string) string {
	return `
id: synthetic
spawn:
  cmd: true
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
model:
  manifest:
    url: ` + url + `
    items_path: "providers.synthetic.models[]"
    item:
      id: "{id}"
      label: "{label}"
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
`
}

// TestManifestFetch_DefaultsDisabledUntilTheSettingsGetterIsWired proves the
// engine-side half of the fetch toggle (agents.go's own doc on
// SetManifestFetchEnabled): a bare New(), never wired to a settings getter,
// must NEVER dial out as a side effect of resolving a descriptor — only the
// embedded/disk halves apply. Wiring a getter that reports true is what
// switches the network half on.
func TestManifestFetch_DefaultsDisabledUntilTheSettingsGetterIsWired(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		// updatedAt must beat the REAL embedded model-manifest.json's own
		// timestamp (ProbeManifest picks whichever candidate is freshest),
		// so far in the future it can never accidentally lose that race.
		_, _ = w.Write([]byte(
			`{"updatedAt":"2099-01-01T00:00:00Z","providers":{"synthetic":{"models":[` +
				`{"id":"m1","label":"M1"}]}}}`,
		))
	}))
	defer server.Close()
	// model.manifest.url must be https: (rules.modelManifest) — httptest's
	// plain server can't satisfy that, so this borrows the TLS variant's own
	// trusting client for the one real net/http call ProbeManifest makes.
	realClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = realClient })

	home := t.TempDir()
	writeDescriptor(t, home, "synthetic", manifestDescriptorYAML(server.URL))
	svc := agents.New()

	a, err := svc.Get(context.Background(), home, "synthetic")
	require.NoError(t, err)
	assert.Empty(t, a.Models(), "no embedded fallback for this id, and fetch defaults disabled")
	assert.Equal(t, 0, calls, "a bare New() must never dial out")

	svc.SetManifestFetchEnabled(func() bool { return true })
	// List, not Get: Get short-circuits on its own descriptor cache when the
	// override file's mtime is unchanged (agents.go's own doc on
	// descriptorCacheEntry), so it would never re-trigger
	// refreshModelsIfDeclared here. List always does. The refresh itself is
	// async, so poll the real signal (the model actually landing) instead
	// of sleeping.
	require.Eventually(t, func() bool {
		list, err := svc.List(context.Background(), home)
		if err != nil {
			return false
		}
		for _, p := range list {
			if p.ID() == "synthetic" {
				return len(p.Models()) == 1
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond)
	assert.Positive(t, calls, "enabling the getter must let the next refresh reach the network")
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
	assert.JSONEq(t, `{"decision":"accept"}`, string(stdout))
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

// A daemon started inside a Claude Code session inherits that session's
// identity; passed on, every chat's CLI would adopt the one session id
// (found by the live conformance run).
func TestAgent_ClaudeNeverInheritsASessionIdentity(t *testing.T) {
	tmp := t.TempDir()
	inherited := []string{
		"PATH=/bin", "CLAUDE_CODE_SESSION_ID=parent", "CLAUDE_CODE_REMOTE_SESSION_ID=parent",
		"CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_PID=1", "CLAUDECODE=1",
	}
	plan, err := get(t, "claude").SpawnPlan(agents.TemplateCtx{
		Tmp: tmp, Segid: "seg", CrowbarHook: "/bin/crowbar", Cwd: tmp,
	}, inherited, nil)
	require.NoError(t, err)
	if plan.Cleanup != nil {
		t.Cleanup(plan.Cleanup)
	}
	assert.Equal(t, []string{"PATH=/bin"}, plan.Env)
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
	_, err := claude.StartAPIConn(context.Background(), "/nonexistent.sock", nil)
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

// TestAgent_APIAttachArgvCarriesConfigInjection pins the sibling gap: the
// attached process is an ORDINARY hooks-transport CLI from the instant it
// starts, and config_injection is where session_start/user_prompt/turn_stop/
// etc. all get wired to {crowbar_hook} — without this, the one process a
// non-hotswap provider hands a live turn over to would report nothing back
// to Crowbar's ledger at all.
func TestAgent_APIAttachArgvCarriesConfigInjection(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-attach-hooks", `
id: api-attach-hooks
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize }
config_injection:
  - pass_arg: { arg: "-c", value: 'hooks.Stop=[{hooks=[{type="command",command="{crowbar_hook} hook turn_stop --segment {segid}"}]}]' }
`)
	a, err := agents.New().Get(context.Background(), home, "api-attach-hooks")
	require.NoError(t, err)

	attachArgv, ok := a.APIAttachArgv(agents.TemplateCtx{
		Socket: "/tmp/s.sock", Session: "sess-1", Segid: "seg-1", CrowbarHook: "/bin/crowbar",
	})
	require.True(t, ok)
	assert.Equal(t, []string{
		"acme", "resume", "sess-1",
		"-c", `hooks.Stop=[{hooks=[{type="command",command="/bin/crowbar hook turn_stop --segment seg-1"}]}]`,
	}, attachArgv, "the attached TUI must be wired with the SAME hooks a normal hooks-transport spawn gets")
}

// TestAgent_APIAttachArgvCarriesSpawnArgs pins a live-confirmed gap: codex's
// switch-to-terminal forks `codex resume {id}` via APIAttachArgv, which never
// applied spawn.args — unlike a normal interactive spawn (spawn.Plan), which
// always puts them right after the executable. Codex's spawn.args carries
// --dangerously-bypass-hook-trust, the only thing that makes codex skip its
// interactive per-hash hook-trust confirmation screen; config_injection (see
// the sibling test above) gives every attached resume a fresh per-segment
// hash, so without this the attached TUI parks on that confirmation screen
// forever instead of reaching the composer. spawn.args are a fact about any
// interactive invocation of the provider's own executable, not about which
// entry point is building the argv — the attach path must carry them exactly
// as the primary spawn path does.
func TestAgent_APIAttachArgvCarriesSpawnArgs(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-attach-spawn-args", `
id: api-attach-spawn-args
spawn:
  cmd: acme
  interactive_required: true
  args:
    - "--trust-workaround"
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize }
`)
	a, err := agents.New().Get(context.Background(), home, "api-attach-spawn-args")
	require.NoError(t, err)

	attachArgv, ok := a.APIAttachArgv(agents.TemplateCtx{Socket: "/tmp/s.sock", Session: "sess-1"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "--trust-workaround", "resume", "sess-1"}, attachArgv,
		"spawn.args must land right after the executable, before the attach subcommand's own args")

	// serve is a genuinely different subcommand (app-server, not an
	// interactive TUI) — spawn.args are interactive-only flags and must NOT
	// leak onto it.
	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{Socket: "/tmp/s.sock"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server", "--listen", "unix:///tmp/s.sock"}, serveArgv,
		"spawn.args are interactive-TUI-only and must not appear on the serve argv")
}

// TestAgent_APIServeArgvCarriesMCPInjection pins the fix for a live-confirmed
// gap: codex's api-transport serve process was never told Crowbar's MCP
// server exists, because forkServeProcess's argv skipped MCPInject/
// ConfigInjection entirely. What a provider needs injected is a fact about
// the provider, not about which of its processes is talking right now — see
// spawn.Inject's own doc comment — so the SAME steps a hooks-attached CLI
// gets via SpawnPlan must also land on the serve argv.
func TestAgent_APIServeArgvCarriesMCPInjection(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-mcp", `
id: api-mcp
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    handshake: { call: initialize }
mcp_injection:
  - pass_arg: { arg: "-c", value: 'mcp_servers.crowbar.command="{crowbar}"' }
  - pass_arg: { arg: "-c", value: 'mcp_servers.crowbar.args=["mcp","--segment","{segid}"]' }
`)
	a, err := agents.New().Get(context.Background(), home, "api-mcp")
	require.NoError(t, err)

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{
		Socket: "/tmp/s.sock", Segid: "seg-1", CrowbarHook: "/bin/crowbar",
	})
	require.True(t, ok)
	assert.Equal(t, []string{
		"acme", "app-server", "--listen", "unix:///tmp/s.sock",
		"-c", `mcp_servers.crowbar.command="/bin/crowbar"`,
		"-c", `mcp_servers.crowbar.args=["mcp","--segment","seg-1"]`,
	}, serveArgv, "the serve process must carry the SAME crowbar MCP registration a hooks-attached CLI gets")
}

func TestAgent_Codex_ServeProcessReportsOverOneChannel(t *testing.T) {
	a := get(t, "codex")
	ctx := agents.TemplateCtx{Socket: "/tmp/s.sock", Cwd: `/work/tree "a"`, CrowbarHook: "/bin/crowbar", Tmp: t.TempDir()}

	serveArgv, ok := a.APIServeArgv(ctx)
	require.True(t, ok)
	plan, err := a.SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)

	assert.NotContains(t, strings.Join(serveArgv, " "), "hooks.", "app-server must not also relay hooks")
	assert.Contains(t, strings.Join(plan.Argv, " "), "hooks.SessionStart=", "the TUI reports over hooks")
	assert.Contains(t, plan.Argv, `projects={"/work/tree \"a\""={trust_level="trusted"}}`,
		"a new worktree must never park the TUI on codex's trust prompt")
	assert.Contains(t, plan.Argv, `tui.resume_cwd="current"`)
}

// TestAgent_APIServeArgvCarriesTheSelection pins the api channel's own
// carrier for a chat's model/effort choice. An api-transport spawn whose
// connection comes up forks NO PTY, so the argv model.apply/effort.apply
// render into never exists; api_apply is the same choice declared onto the
// serve process instead. Without it the choice is built and thrown away.
func TestAgent_APIServeArgvCarriesTheSelection(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-selection", `
id: api-selection
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    handshake: { call: initialize }
model:
  available: [fast, deep]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
  api_apply:
    - pass_arg: { arg: "-c", value: 'model="{model}"' }
effort:
  available: { "*": [low, high] }
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
  api_apply:
    - pass_arg: { arg: "-c", value: 'reasoning="{effort}"' }
`)
	a, err := agents.New().Get(context.Background(), home, "api-selection")
	require.NoError(t, err)

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{
		Socket: "/tmp/s.sock", Model: "deep", Effort: "high",
	})
	require.True(t, ok)
	assert.Equal(t, []string{
		"acme", "app-server", "--listen", "unix:///tmp/s.sock",
		"-c", `model="deep"`,
		"-c", `reasoning="high"`,
	}, serveArgv)
	assert.NotContains(t, serveArgv, "--model",
		"the argv carrier is for a forked PTY; the serve process takes the api one")
}

// A chat that chose nothing renders an argv byte-identical to one built
// before the feature existed — the same guarantee SelectionSteps already
// makes for the forked path.
func TestAgent_APIServeArgvIsUnchangedWhenNothingWasChosen(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-selection-empty", `
id: api-selection-empty
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+`
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve:  [acme, app-server, --listen, "unix://{socket}"]
    handshake: { call: initialize }
model:
  available: [fast, deep]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
  api_apply:
    - pass_arg: { arg: "-c", value: 'model="{model}"' }
`)
	a, err := agents.New().Get(context.Background(), home, "api-selection-empty")
	require.NoError(t, err)

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{Socket: "/tmp/s.sock"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server", "--listen", "unix:///tmp/s.sock"}, serveArgv)
}

// TestRegression_ACodexChatsChosenModelReachesItsServeProcess is the reported
// defect at the descriptor layer: a codex chat's model/effort rode
// model.apply/effort.apply only, and codex's api-transport spawn forks no PTY
// to put them on — so the user's choice silently reverted to the provider
// default for the life of the chat.
//
// Driven against the REAL shipped descriptor, which is the only place the key
// names live. Verified against codex-cli 0.154.0: `codex app-server --help`
// documents `-c model="o3"` as its first example and offers no --model flag
// at all, which is why the api carrier is the config channel and not a
// second copy of the TUI's own.
func TestRegression_ACodexChatsChosenModelReachesItsServeProcess(t *testing.T) {
	serveArgv, ok := get(t, "codex").APIServeArgv(agents.TemplateCtx{
		Socket: "/tmp/s.sock", Model: "gpt-5.4-codex", Effort: "high",
	})

	require.True(t, ok)
	assert.Contains(t, serveArgv, `model="gpt-5.4-codex"`,
		"an adopted codex connection has no argv but this one; a model that misses it is not run")
	assert.Contains(t, serveArgv, `model_reasoning_effort="high"`)
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
