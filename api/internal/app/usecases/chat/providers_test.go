package chat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// ─── from providers_test.go ───────────────────────────────────────────

// geminiDescriptor is a minimal-valid on-disk descriptor override, written into a
// fixture home so ResolveProviders sees a THIRD provider (beyond the embedded
// claude/codex) with no stored preference — the "appended by id, enabled by
// default" case.
const geminiDescriptor = `id: gemini
display_name: Gemini
icon: '<svg/>'
spawn:
  cmd: gemini
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func writeGeminiDescriptor(
	t *testing.T,
	home string,
) {
	t.Helper()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gemini.yaml"), []byte(geminiDescriptor), 0o644))
}

func providerIDs(
	got []domain.AgentProvider,
) []string {
	ids := make([]string, len(got))
	for i, p := range got {
		ids[i] = p.ID
	}
	return ids
}

// TestResolveProviders_OrdersByPreferenceThenAppendsNewEnabled pins the whole
// resolution contract: preferenced providers come first in saved priority order,
// unpreferenced descriptors are appended by id, enabled reflects !disabled, and a
// provider with no row defaults to enabled. Connected is the injected probe's
// verdict, independent of the host.
func TestResolveProviders_OrdersByPreferenceThenAppendsNewEnabled(t *testing.T) {
	f := newFixture(t)
	writeGeminiDescriptor(t, f.ws.home)

	f.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Priority: 0, Disabled: false},
		domain.AgentProviderPreference{ProviderID: "claude", Priority: 1, Disabled: true},
	)
	f.setConnected(map[string]bool{"codex": true, "claude": false, "gemini": true})

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"codex", "claude", "gemini"}, providerIDs(got),
		"preferenced first in priority order, unpreferenced appended by id")
	assert.True(t, got[0].Enabled, "codex enabled")
	assert.True(t, got[0].Connected, "codex installed")
	assert.False(t, got[1].Enabled, "claude disabled")
	assert.False(t, got[1].Connected, "claude not installed")
	assert.True(t, got[2].Enabled, "gemini defaults to enabled")
	assert.True(t, got[2].Connected, "gemini installed")
}

// TestResolveProviders_NoPreferences_OrdersByDescriptorID proves the empty-store
// default: with no stored preferences every embedded descriptor is enabled and
// ordered by id.
func TestResolveProviders_NoPreferences_OrdersByDescriptorID(t *testing.T) {
	f := newFixture(t)
	f.setConnected(map[string]bool{"claude": true, "codex": true})

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"claude", "codex"}, providerIDs(got))
	for _, p := range got {
		assert.True(t, p.Enabled, "%s defaults to enabled", p.ID)
	}
}

// A disabled provider is a provider the user has switched OFF, and the only
// place that decision can be honoured is the spawn path: the preference is
// persisted and reported, but a POST .../chats naming it — from a stale
// tab, a second window, or the CLI — reaches spawnRunner directly and never
// passes through the list the Enabled flag decorates.
func TestSpawnChat_RefusesDisabledProvider(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", Disabled: true})

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "codex")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument,
		"a disabled provider must be refused with a 4xx-mapping sentinel")
	assert.Zero(t, f.term.callCount(), "no vendor CLI may be launched for a disabled provider")
}

// The guard is scoped to the provider that is actually off: disabling one must
// not take the others down with it.
func TestSpawnChat_AllowsEnabledProviderAlongsideADisabledOne(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Disabled: true},
		domain.AgentProviderPreference{ProviderID: "claude", Disabled: false},
	)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.NoError(t, err)
	assert.Equal(t, 1, f.term.callCount())
}

// Switching an existing chat onto a disabled provider is the same decision by
// another route, and it is the worse one to leave open: the switch QUITS the
// live CLI before it spawns the replacement, so an unguarded switch onto a
// disabled provider would leave the chat with no agent at all.
func TestSwitchProvider_RefusesDisabledTarget(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", Disabled: true})

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Equal(t, 1, f.term.callCount(), "the disabled target must never be spawned")
	assert.Empty(t, f.term.terminateRequestIDs(),
		"a refused switch must not quit the CLI the chat still has")
}

// ─── from mcp_test.go ─────────────────────────────────────────────────

// mcpResult decodes a JSON-RPC response's result member, failing the test on a
// JSON-RPC error (which is a TRANSPORT fault here — a tool that merely failed
// comes back as a successful result carrying isError).
func mcpResult(
	t *testing.T,
	raw []byte,
	into any,
) {
	t.Helper()
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Nil(t, resp.Error, "unexpected JSON-RPC error: %s", resp.Error)
	require.NoError(t, json.Unmarshal(resp.Result, into))
}

type mcpCallResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// TestDispatchMCP_InitializeAnnouncesCrowbar proves the seam builds a server
// that speaks the pinned protocol revision and identifies itself as crowbar —
// the handshake every vendor CLI's MCP client performs before it will call a
// tool.
func TestDispatchMCP_InitializeAnnouncesCrowbar(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	mcpResult(t, out, &res)
	assert.Equal(t, enginemcp.ProtocolVersion, res.ProtocolVersion)
	assert.Equal(t, "crowbar", res.ServerInfo.Name)
}

// TestDispatchMCP_ListsTheChatTools proves the usecase wires itself in as BOTH
// the tool surface's ChatRenamer (Deps.Chats) and its ChatLogReader
// (Deps.ChatLogs). agenttools registers nothing for a nil dependency, so a
// build that failed to make either self-assignment would still answer
// tools/list successfully — just with set_chat_title or get_chat_log quietly
// missing — and nothing else would ever notice.
//
// This asserts the FULL 8-name production surface, not merely Contains, and it
// does so against the REAL concerns built through the real agentusecase.New (via
// newFixture → newFixtureUsing), never a Deps struct a test hand-assembles and
// then patches — the latter can only prove the surface WOULD be complete if
// New's own self-assignment ran, not that it actually did. That distinction is
// exactly what a deleted `tools.Chats = chat` or `tools.ChatLogs = chat` needs to
// be caught by something.
func TestDispatchMCP_ListsTheChatTools(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	require.True(t, send)

	var res struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	mcpResult(t, out, &res)

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"set_chat_title",
		"list_review_threads",
		"get_review_scope",
		"post_review_comment",
		"reply_to_review_thread",
		"resolve_review_thread",
		"list_workspaces",
		"get_chat_log",
	}, names, "a real provider concern must advertise the complete production tool surface")
}

// TestDispatchMCP_ToolCallRenamesTheCallersChat is the end-to-end proof of the
// seam: an authenticated tools/call reaches a handler, and that handler's effect
// lands on the chat the CALLING runner is on right now — resolved from the
// runner, never from an argument, which is why no tool takes a chat id.
func TestDispatchMCP_ToolCallRenamesTheCallersChat(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Fix The Parser"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.False(t, res.IsError, "tool reported an error: %+v", res.Content)

	assert.Equal(t, "Fix The Parser", f.chat(t, chatID).Title)
}

// TestDispatchMCP_RejectsAnotherRunnersToken is the authority test: holding a
// token minted for a DIFFERENT runner must not let a caller act as this one. The
// refusal comes back as a tool error rather than a transport error — that is how
// the model is told — but the write must not happen.
func TestDispatchMCP_RejectsAnotherRunnersToken(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, otherRunnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(otherRunnerID),
		[]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Stolen"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	assert.True(t, res.IsError, "a token minted for another runner must not authenticate")
	assert.Empty(t, f.chat(t, chatID).Title, "the rejected call must not have written")
}

// TestDispatchMCP_NotificationIsSilent proves a JSON-RPC notification produces
// no reply at all, which the HTTP seam turns into 204.
func TestDispatchMCP_NotificationIsSilent(
	t *testing.T,
) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	require.NoError(t, err)
	assert.False(t, send)
	assert.Empty(t, out)
}

// TestDispatchMCP_UnconfiguredSurfaceIsAnError proves a usecase built without a
// tool surface REFUSES the call instead of answering with an empty tool list. A
// wiring mistake has to be loud: an agent that silently has no tools looks
// identical to an agent that chose not to use them.
func TestDispatchMCP_UnconfiguredSurfaceIsAnError(
	t *testing.T,
) {
	u := agentusecase.New(agentusecase.Deps{})

	_, send, err := u.DispatchMCP(t.Context(), "seg-1", "tok",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.Error(t, err)
	assert.False(t, send)
}

// TestSpawnChat_MintsARunnerTokenTheDispatchSeamAccepts closes the loop the
// descriptor tests can only render half of: the token baked into the CLI's argv at
// spawn must be one THIS daemon's minter verifies. A spawn path that minted from a
// different secret — or shipped the segment id twice — would render argv that looks
// perfectly right and authenticate nothing.
func TestSpawnChat_MintsARunnerTokenTheDispatchSeamAccepts(
	t *testing.T,
) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	var config struct {
		MCPServers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw := argAfter(t, f.term.calls[0].argv, "--mcp-config")
	require.NoError(t, json.Unmarshal([]byte(raw), &config), "rendered --mcp-config is not valid JSON: %s", raw)

	args := config.MCPServers["crowbar"].Args
	require.Equal(t, runnerID, args[indexOf(args, "--segment")+1])
	token := args[indexOf(args, "--token")+1]
	require.NotEmpty(t, token)
	require.NotEqual(t, runnerID, token, "the token must not be the segment id the agent can already read")

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, token,
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Minted At Spawn"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.False(t, res.IsError, "the token the CLI was actually given must authenticate: %+v", res.Content)
	assert.Equal(t, "Minted At Spawn", f.chat(t, chatID).Title)
}

// TestSpawnChat_InjectsTheCapabilityPreamble: registering the MCP server is only
// half the job. A model asked to "review this branch" reaches for gh or writes
// prose unless it is TOLD which surface to prefer — tool descriptions alone do not
// override that prior — so the preamble travels in the same {context} document
// every other injected instruction uses.
func TestSpawnChat_InjectsTheCapabilityPreamble(
	t *testing.T,
) {
	f := newFixture(t)
	f.spawn(t, "claude")

	doc := argAfter(t, f.term.calls[0].argv, "--append-system-prompt")
	assert.Contains(t, doc, "Crowbar workspace")
	assert.Contains(t, doc, "prefer them over shell equivalents")

	// The titling ASK, not just the titling tool. Retiring title_instruction took
	// the request away with the shell command, and set_chat_title then went
	// uncalled on both providers: a tool description says what a tool does, only
	// this says a title is wanted. Titling is proactive work unrelated to the
	// user's actual request, so nothing happens unless something asks.
	assert.Contains(t, doc, "set_chat_title")
}

// TestSpawnChat_Codex_InjectsTheCapabilityPreamble is codex's counterpart: it ships
// no --append-system-prompt, so the preamble rides the same silent
// developer_instructions channel a handoff uses. It must never arrive as a
// positional, which IS codex's opening user message.
//
// Together with TestResumeChat_GaplessRevive_SendsNoUserMessage below this pins the
// whole delivery rule: a FRESH spawn is injected even when it has no handoff to
// carry (the preamble is the point), a gapless RESUME is not injected at all.
func TestSpawnChat_Codex_InjectsTheCapabilityPreamble(
	t *testing.T,
) {
	f := newFixture(t)
	f.spawn(t, "codex")

	argv := f.term.calls[0].argv
	devInstructions := configValue(t, argv, "developer_instructions=")
	assert.Contains(t, devInstructions, "Crowbar workspace")
	assert.Contains(t, devInstructions, "set_chat_title")

	for i, a := range argv {
		if i > 0 && argv[i-1] == "-c" {
			continue
		}
		assert.NotContains(t, a, "Crowbar workspace",
			"the preamble must not be injected as a codex positional arg")
	}
}

// TestComposeContext_PreambleLeadsAndAbsentHalvesLeaveNoGap pins the {context}
// document's shape at the one place it can be stated exactly.
//
// The preamble LEADS: it says which tools this CLI has and when to prefer them, and a
// model should read that before a handoff it is explicitly told not to act on. Either
// half can be missing — a user may blank capabilities_instruction in their own
// config.yaml, and a brand-new chat has no conversation — and a missing half must
// leave no stray blank line, which a model reads as a section that was meant to be
// there.
func TestComposeContext_PreambleLeadsAndAbsentHalvesLeaveNoGap(
	t *testing.T,
) {
	cases := []struct {
		name         string
		preamble     string
		conversation string
		wantDocument string
	}{{
		name:         "a fresh chat has only the preamble",
		preamble:     "PREAMBLE",
		wantDocument: "PREAMBLE",
	}, {
		name:         "a chat taking a handoff, with the preamble leading",
		preamble:     "PREAMBLE",
		conversation: "HANDOFF",
		wantDocument: "PREAMBLE\n\nHANDOFF",
	}, {
		name:         "capabilities_instruction blanked in the user's own config.yaml",
		conversation: "HANDOFF",
		wantDocument: "HANDOFF",
	}, {
		name: "nothing to say",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantDocument,
				agentusecase.ComposeContext(tc.preamble, tc.conversation))
		})
	}
}

// TestResumeChat_GaplessRevive_SendsNoUserMessage is the regression the capability
// preamble could very easily have caused. Reopening a closed tab resumes the same
// provider with nothing recorded in between, and a resumed codex can be reached
// ONLY through a user message (see codex.yaml) — so if the preamble were allowed to
// make the {context} document non-empty on its own, every reopened codex chat would
// open with a "while you were away" pointer about nothing that happened, and codex
// answers its opening message on sight.
func TestResumeChat_GaplessRevive_SendsNoUserMessage(
	t *testing.T,
) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sid-codex-native")
	turn(t, f, runnerID, "codex", "codex said something")
	require.NoError(t, f.usecase.RenameChat(f.ctx, chatID, "Already Named", "user"))
	f.wait()

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	require.Equal(t, 2, f.term.callCount())
	for i, a := range f.term.calls[1].argv {
		if i > 0 && f.term.calls[1].argv[i-1] == "-c" {
			continue
		}
		assert.NotContains(t, a, "While you were away",
			"a revive with no gap must hand the CLI no user message at all")
	}
}

// ─── from mcp_toggle_test.go ──────────────────────────────────────────

// configKeys returns the KEY of every `-c key=value` override in a codex argv.
//
// Parsing rather than grepping is the point. `require.NotContains(joined,
// "mcp_servers")` passes for a great many argvs that are still wrong — an argv
// where the key survived with a mangled value, or where one of the four MCP
// overrides was dropped and three were not — and it is exactly the assertion that
// would let a partial registration through. The key SET is the fact being
// asserted, so the key set is what the test reads.
func configKeys(argv []string) []string {
	var out []string
	for i, a := range argv {
		if i == 0 || argv[i-1] != "-c" {
			continue
		}
		key, _, ok := strings.Cut(a, "=")
		if !ok {
			continue
		}
		out = append(out, key)
	}
	return out
}

// mcpConfigJSON reports whether any argv token is a claude --mcp-config document,
// read as JSON rather than searched for as text: an argv that carried the
// registration under a different flag, or in a token whose quoting had come apart,
// is still a registration and must still fail this.
func hasMCPServers(argv []string) bool {
	for _, a := range argv {
		var doc struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if json.Unmarshal([]byte(a), &doc) == nil && len(doc.MCPServers) > 0 {
			return true
		}
	}
	return false
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// spawnArgvShape spawns provider under an optional stored preference and returns
// the CLI's argv with everything per-spawn substituted out — the runner id (a
// fresh uuid that rides in every hook command and in the MCP registration), the
// token minted for it, and the fixture's crowbar home (a fresh temp dir).
//
// What is left is the sequence of steps that were rendered, which is the thing
// two spawns can meaningfully be compared on.
func spawnArgvShape(
	t *testing.T,
	provider string,
	pref *domain.AgentProviderPreference,
) []string {
	t.Helper()
	f := newFixture(t)
	if pref != nil {
		f.setPrefs(t, *pref)
	}
	_, runnerID := f.spawn(t, provider)
	repl := strings.NewReplacer(
		f.ws.home, "<HOME>",
		runnerID, "<RUNNER>",
		f.minter.Mint(runnerID), "<TOKEN>",
	)
	out := make([]string, 0, len(f.term.calls[0].argv))
	for _, a := range f.term.calls[0].argv {
		out = append(out, repl.Replace(a))
	}
	return out
}

// codexMCPKeys is every `-c` key codex's mcp_injection registers. Named here so
// the disabled-case assertion is about ALL of them and cannot quietly become an
// assertion about whichever one a later edit left behind.
var codexMCPKeys = []string{
	"mcp_servers.crowbar.command",
	"mcp_servers.crowbar.args",
	"mcp_servers.crowbar.env_vars",
	"mcp_servers.crowbar.default_tools_approval_mode",
}

// With the tool surface switched off, NOTHING of the MCP registration may reach
// the CLI. A half-registration is the specific failure to design against: codex
// carries {runner_token} in exactly one of its four MCP overrides, so any scheme
// that filtered templated steps would leave .command, .env_vars and
// .default_tools_approval_mode behind and register a server with no arguments —
// a broken tool surface rather than an absent one.
func TestSpawnChat_MCPDisabled_RendersNoRegistrationAtAll(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		f := newFixture(t)
		f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", MCPDisabled: true})

		f.spawn(t, "claude")
		argv := f.term.calls[0].argv

		assert.Equal(t, -1, indexOf(argv, "--mcp-config"))
		assert.False(t, hasMCPServers(argv), "an mcpServers document reached claude: %q", argv)
		// The registration must not have relocated into the hook settings file
		// either — that file is written on every spawn, MCP or not.
		settings := readFile(t, argAfter(t, argv, "--settings"))
		assert.NotContains(t, settings, "mcpServers")
	})

	t.Run("codex", func(t *testing.T) {
		f := newFixture(t)
		f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", MCPDisabled: true})

		f.spawn(t, "codex")
		keys := configKeys(f.term.calls[0].argv)

		for _, key := range codexMCPKeys {
			assert.NotContains(t, keys, key,
				"codex was still handed %s with the tool surface switched off", key)
		}
		// And not by some other spelling: no override under the mcp_servers tree
		// at all.
		for _, key := range keys {
			assert.False(t, strings.HasPrefix(key, "mcp_servers."),
				"unexpected MCP override %q survived", key)
		}
	})
}

// The two switches are different switches. Disabling a provider stops the spawn;
// disabling its tool surface must not — the CLI comes up, the chat works, and the
// hooks Crowbar depends on for every turn keep firing. A toggle that quietly
// broke the chat would be worse than no toggle.
func TestSpawnChat_MCPDisabled_TheCLIStillSpawnsAndItsHooksStillFire(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			f := newFixture(t)
			f.setPrefs(t, domain.AgentProviderPreference{ProviderID: provider, MCPDisabled: true})

			chatID, runnerID := f.spawn(t, provider)
			require.Equal(t, 1, f.term.callCount(), "the CLI must still be spawned")

			// The hook wiring is still in the argv the CLI was handed: claude reads
			// it from the settings file, codex from -c overrides. Assert the actual
			// channel each one uses, because "the flag is present" is not the same
			// claim as "the hooks are configured".
			argv := f.term.calls[0].argv
			if provider == "claude" {
				assert.Contains(t, readFile(t, argAfter(t, argv, "--settings")), "session_start")
			} else {
				assert.Contains(t, configValue(t, argv, "hooks.SessionStart="), "session_start")
			}

			// And they FIRE: a session binds and a turn lands in the ledger, driven
			// through IngestHook exactly as the real CLI drives it.
			f.announce(t, runnerID, "sid-"+provider)
			assert.Equal(t, "sid-"+provider, f.runner(t, runnerID).CurrentSession)

			turn(t, f, runnerID, provider, "still talking")
			assert.Equal(t, chatID, f.chatForSession(t, "sid-"+provider))
		})
	}
}

// A stored row that predates the column keeps its tool surface. The column is
// negative precisely so AutoMigrate's backfill of false reads as "enabled" — a
// positive flag would have switched MCP off for every provider the user had ever
// reordered or disabled, the first time the daemon came up on the new schema.
func TestSpawnChat_APreferenceRowWithNoExplicitValueKeepsMCPEnabled(t *testing.T) {
	f := newFixture(t)
	// Exactly the row the old PUT body wrote: a priority and an enabled flag, and
	// nothing about MCP.
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", Priority: 0, Disabled: false})

	f.spawn(t, "claude")

	assert.GreaterOrEqual(t, indexOf(f.term.calls[0].argv, "--mcp-config"), 0,
		"a preference row written before the column existed must not lose the tool surface")

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)
	for _, p := range got {
		assert.True(t, p.MCPEnabled, "%s reports its tool surface off", p.ID)
	}
}

// With the switch ON, the spawn must be what it was before the switch existed.
// The comparison is against a spawn with NO preference row at all — the state
// every provider was in before this task — so "identical" is a fact the test can
// actually establish rather than a claim about a previous commit.
func TestSpawnChat_MCPEnabled_IsIdenticalToNoPreferenceAtAll(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			assert.Equal(t,
				spawnArgvShape(t, provider, nil),
				spawnArgvShape(t, provider,
					&domain.AgentProviderPreference{ProviderID: provider, MCPDisabled: false}))
		})
	}
}

// The switch is PER PROVIDER: turning codex's tools off must leave claude's alone.
// A global flag would satisfy every assertion above and still be the wrong feature.
func TestSpawnChat_MCPDisabled_IsScopedToTheProviderItNames(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", MCPDisabled: true})

	f.spawn(t, "codex")
	f.spawn(t, "claude")

	assert.NotContains(t, configKeys(f.term.calls[0].argv), "mcp_servers.crowbar.command")
	assert.GreaterOrEqual(t, indexOf(f.term.calls[1].argv, "--mcp-config"), 0,
		"claude's tool surface must survive codex's switch")

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)
	byID := map[string]bool{}
	for _, p := range got {
		byID[p.ID] = p.MCPEnabled
	}
	assert.False(t, byID["codex"])
	assert.True(t, byID["claude"])
}

// Switching the tool surface off must not switch the PROVIDER off. They are
// stored in adjacent columns and read a few lines apart, which is exactly the
// shape of mistake that spawns nothing and reports a disabled provider.
func TestSpawnChat_MCPDisabledDoesNotDisableTheProvider(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", MCPDisabled: true})

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "codex")
	require.NoError(t, err)

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)
	for _, p := range got {
		if p.ID == "codex" {
			assert.True(t, p.Enabled, "the provider itself must stay enabled")
			assert.False(t, p.MCPEnabled)
		}
	}
}

// ── the switch is a LIVE permission, not a spawn-time one ───────────
// Rendering the registration out of a CLI's argv only governs chats that have
// not started yet. The CLI holds its MCP stdio channel for the life of the
// process, so a chat spawned while the switch was ON keeps a working tool
// surface for as long as it runs — unless the DISPATCH consults the preference
// too. The UI calls this switch "Tools" and describes it as letting a provider's
// agent use Crowbar itself; that sentence is only true if the daemon refuses.

// TestDispatchMCP_ToolCallIsRefusedOnceToolsAreSwitchedOff is the live-gate
// guard. It spawns with the switch on (so the CLI is genuinely holding a
// registered tool surface), turns the switch off underneath it, and calls a
// tool: the refusal must come back as a tool error and the write must not
// happen.
func TestDispatchMCP_ToolCallIsRefusedOnceToolsAreSwitchedOff(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	// The chat was spawned with tools ON — the registration reached the CLI.
	require.GreaterOrEqual(t, indexOf(f.term.calls[0].argv, "--mcp-config"), 0)

	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", MCPDisabled: true})

	out, send, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Should Not Land"}}}`))
	require.NoError(t, err)
	require.True(t, send)

	var res mcpCallResult
	mcpResult(t, out, &res)
	assert.True(t, res.IsError, "a tool call from a provider with its tools off must be refused")
	assert.Empty(t, f.chat(t, chatID).Title, "the refused call must not have written")
}

// The refusal is not a mystery to the model. A tool error rides back as result
// text, so it has to say WHICH switch is off and where it lives — otherwise the
// model reads a bare failure and retries the same call for the rest of the turn.
func TestDispatchMCP_ToolRefusalNamesTheSwitchThatIsOff(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", MCPDisabled: true})

	out, _, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"x"}}}`))
	require.NoError(t, err)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	assert.Contains(t, res.Content[0].Text, "switched off")
	assert.Contains(t, res.Content[0].Text, "Providers")
}

// The gate is PER PROVIDER, exactly like the spawn-time half: switching codex's
// tools off must not refuse a claude runner's calls. A global refusal would pass
// both tests above and still be the wrong feature.
func TestDispatchMCP_TheLiveGateIsScopedToTheProviderItNames(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", MCPDisabled: true})

	out, _, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
		[]byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Still Allowed"}}}`))
	require.NoError(t, err)

	var res mcpCallResult
	mcpResult(t, out, &res)
	require.False(t, res.IsError, "claude's tools must survive codex's switch: %+v", res.Content)
	assert.Equal(t, "Still Allowed", f.chat(t, chatID).Title)
}

// And the switch coming back ON restores the surface within the same running
// chat — the other direction of the same property, and the one that proves the
// preference is genuinely re-read per call rather than cached after the first
// refusal.
func TestDispatchMCP_SwitchingToolsBackOnRestoresThemInARunningChat(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", MCPDisabled: true})

	call := func(title string) mcpCallResult {
		t.Helper()
		out, _, err := f.usecase.DispatchMCP(f.ctx, runnerID, f.minter.Mint(runnerID),
			[]byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":`+
				`{"name":"set_chat_title","arguments":{"title":"`+title+`"}}}`))
		require.NoError(t, err)
		var res mcpCallResult
		mcpResult(t, out, &res)
		return res
	}

	require.True(t, call("Refused").IsError)

	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", MCPDisabled: false})

	require.False(t, call("Allowed Again").IsError)
	assert.Equal(t, "Allowed Again", f.chat(t, chatID).Title)
}
