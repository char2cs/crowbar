package agent_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
