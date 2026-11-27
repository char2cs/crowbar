package chat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

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
  user_prompt: { in: user_prompt, map: { message: prompt } }
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
hooks_injection: [{ pass_arg: { arg: "--hook", value: "{hook_curl} {hook_url}/session_start" } }]
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

func TestResolveProviders_OrdersByPreferenceThenAppendsNewEnabled(t *testing.T) {
	f := newFixture(t)
	writeGeminiDescriptor(t, f.ws.home)
	all, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)
	rest := slices.DeleteFunc(providerIDs(all), func(id string) bool { return id == "codex" || id == "claude" })

	f.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Priority: 0, Disabled: false},
		domain.AgentProviderPreference{ProviderID: "claude", Priority: 1, Disabled: true},
	)
	f.setConnected(map[string]bool{"codex": true, "claude": false, "gemini": true})

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	assert.Equal(t, append([]string{"codex", "claude"}, rest...), providerIDs(got),
		"preferenced first in priority order, unpreferenced appended by id")
	assert.True(t, got[0].Enabled, "codex enabled")
	assert.True(t, got[0].Connected, "codex installed")
	assert.False(t, got[1].Enabled, "claude disabled")
	assert.False(t, got[1].Connected, "claude not installed")
	gemini := got[slices.Index(providerIDs(got), "gemini")]
	assert.True(t, gemini.Enabled, "gemini defaults to enabled")
	assert.True(t, gemini.Connected, "gemini installed")
}

func TestResolveProviders_NoPreferences_OrdersByDescriptorID(t *testing.T) {
	f := newFixture(t)
	f.setConnected(map[string]bool{"claude": true, "codex": true})

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	assert.True(t, slices.IsSorted(providerIDs(got)), "ordered by descriptor id")
	assert.Subset(t, providerIDs(got), []string{"claude", "codex"})
	for _, p := range got {
		assert.True(t, p.Enabled, "%s defaults to enabled", p.ID)
	}
}

func TestStartRunner_RefusesDisabledProvider(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", Disabled: true})

	_, _, err := f.spawnChat(f.ctx, "codex")

	require.ErrorIs(t, err, problem.ErrInvalidArgument,
		"a disabled provider must be refused with a 4xx-mapping sentinel")
	assert.Zero(t, f.term.callCount(), "no vendor CLI may be launched for a disabled provider")
}

func TestStartRunner_AllowsEnabledProviderAlongsideADisabledOne(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Disabled: true},
		domain.AgentProviderPreference{ProviderID: "claude", Disabled: false},
	)

	_, _, err := f.spawnChat(f.ctx, "claude")

	require.NoError(t, err)
	assert.Equal(t, 1, f.term.callCount())
}

func TestSwitchProvider_RefusesDisabledTarget(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", Disabled: true})

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	assert.Equal(t, 1, f.term.callCount(), "the disabled target must never be spawned")
	assert.Empty(t, f.term.terminateRequestIDs(),
		"a refused switch must not quit the CLI the chat still has")
}

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
		"branch_out",
		"list_review_threads",
		"wait_for_review_comments",
		"get_review_scope",
		"post_review_comment",
		"reply_to_review_thread",
		"resolve_review_thread",
		"list_workspaces",
		"get_chat_log",
	}, names, "a real provider concern must advertise the complete production tool surface")
}

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

func TestDispatchMCP_UnconfiguredSurfaceIsAnError(
	t *testing.T,
) {
	u := agentusecase.New(agentusecase.Deps{Ledger: newLedger(t)})

	_, send, err := u.DispatchMCP(t.Context(), "seg-1", "tok",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.Error(t, err)
	assert.False(t, send)
}

func TestStartRunner_MintsARunnerTokenTheDispatchSeamAccepts(
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
	require.Equal(t, runnerID, args[slices.Index(args, "--segment")+1])
	written, err := os.ReadFile(args[slices.Index(args, "--token-file")+1])
	require.NoError(t, err)
	token := strings.TrimSpace(string(written))
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

func TestStartRunner_InjectsTheCapabilityPreamble(
	t *testing.T,
) {
	f := newFixture(t)
	f.spawn(t, "claude")

	doc := argAfter(t, f.term.calls[0].argv, "--append-system-prompt")
	assert.Contains(t, doc, "Crowbar workspace")
	assert.Contains(t, doc, "prefer them over shell equivalents")

	assert.Contains(t, doc, "set_chat_title")
}

func TestStartRunner_Codex_InjectsTheCapabilityPreamble(
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
	chatID, runnerID := f.spawn(t, provider)
	pairs := []string{f.ws.home, "<HOME>", runnerID, "<RUNNER>", chatID, "<CHAT>", f.minter.Mint(runnerID), "<TOKEN>"}
	if session := f.conversation(t, chatID, provider).SessionID; session != "" {
		pairs = append(pairs, session, "<SESSION>")
	}
	repl := strings.NewReplacer(pairs...)
	out := make([]string, 0, len(f.term.calls[0].argv))
	for _, a := range f.term.calls[0].argv {
		out = append(out, repl.Replace(a))
	}
	return out
}

var codexMCPKeys = []string{
	"mcp_servers.crowbar.command",
	"mcp_servers.crowbar.args",
	"mcp_servers.crowbar.env_vars",
	"mcp_servers.crowbar.default_tools_approval_mode",
}

func TestStartRunner_MCPDisabled_RendersNoRegistrationAtAll(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		f := newFixture(t)
		f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", MCPDisabled: true})

		f.spawn(t, "claude")
		argv := f.term.calls[0].argv

		assert.Equal(t, -1, slices.Index(argv, "--mcp-config"))
		assert.False(t, hasMCPServers(argv), "an mcpServers document reached claude: %q", argv)
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
		for _, key := range keys {
			assert.False(t, strings.HasPrefix(key, "mcp_servers."),
				"unexpected MCP override %q survived", key)
		}
	})
}

func TestStartRunner_MCPDisabled_TheCLIStillSpawnsAndItsHooksStillFire(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			f := newFixture(t)
			f.setPrefs(t, domain.AgentProviderPreference{ProviderID: provider, MCPDisabled: true})

			chatID, runnerID := f.spawn(t, provider)
			require.Equal(t, 1, f.term.callCount(), "the CLI must still be spawned")

			argv := f.term.calls[0].argv
			if provider == "claude" {
				assert.Contains(t, readFile(t, argAfter(t, argv, "--settings")), "session_start")
			} else {
				assert.Contains(t, configValue(t, argv, "hooks.SessionStart="), "session_start")
			}

			f.announce(t, runnerID, "sid-"+provider)
			assert.Equal(t, "sid-"+provider, f.conversation(t, chatID, provider).SessionID)

			turn(t, f, runnerID, provider, "still talking")
			assert.Equal(t, chatID, f.chatForSession(t, provider, "sid-"+provider))
		})
	}
}

func TestStartRunner_APreferenceRowWithNoExplicitValueKeepsMCPEnabled(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "claude", Priority: 0, Disabled: false})

	f.spawn(t, "claude")

	assert.GreaterOrEqual(t, slices.Index(f.term.calls[0].argv, "--mcp-config"), 0,
		"a preference row written before the column existed must not lose the tool surface")

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)
	for _, p := range got {
		assert.True(t, p.MCPEnabled, "%s reports its tool surface off", p.ID)
	}
}

func TestStartRunner_MCPEnabled_IsIdenticalToNoPreferenceAtAll(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			assert.Equal(t,
				spawnArgvShape(t, provider, nil),
				spawnArgvShape(t, provider,
					&domain.AgentProviderPreference{ProviderID: provider, MCPDisabled: false}))
		})
	}
}

func TestStartRunner_MCPDisabled_IsScopedToTheProviderItNames(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", MCPDisabled: true})

	f.spawn(t, "codex")
	f.spawn(t, "claude")

	assert.NotContains(t, configKeys(f.term.calls[0].argv), "mcp_servers.crowbar.command")
	assert.GreaterOrEqual(t, slices.Index(f.term.calls[1].argv, "--mcp-config"), 0,
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

func TestStartRunner_MCPDisabledDoesNotDisableTheProvider(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", MCPDisabled: true})

	_, _, err := f.spawnChat(f.ctx, "codex")
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

func TestDispatchMCP_ToolCallIsRefusedOnceToolsAreSwitchedOff(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	require.GreaterOrEqual(t, slices.Index(f.term.calls[0].argv, "--mcp-config"), 0)

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
