package agent_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestResolveDescriptor_EmbeddedClaudeValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Equal(t, "claude", d.ID)
	require.Equal(t, "claude", d.Spawn.Cmd)
	require.True(t, d.Spawn.InteractiveRequired)
	require.Contains(t, d.Spawn.ForbidFlags, "-p")
	require.Equal(t, "json", d.Hooks.Format)
	require.Equal(t, "session_id", d.Hooks.Events["session_start"]["session_id"])
	require.Equal(t, "last_assistant_message", d.Hooks.Events["turn_stop"]["message"])

	// claude reads an appended system prompt without being given a turn, and
	// honours a FRESH one on --resume too, so the same silent channel carries
	// {context} whether the session is new or resumed.
	require.Len(t, d.ContextInject, 1)
	require.Equal(t, "pass_arg", d.ContextInject[0].Verb)
	require.Equal(t, "--append-system-prompt", d.ContextInject[0].Args["arg"])
	require.Equal(t, "{context}", d.ContextInject[0].Args["value"])

	require.Len(t, d.ResumeContextInject, 1)
	require.Equal(t, "pass_arg", d.ResumeContextInject[0].Verb)
	require.Equal(t, "--append-system-prompt", d.ResumeContextInject[0].Args["arg"])
	require.Equal(t, "{context}", d.ResumeContextInject[0].Args["value"])
}

func TestResolveDescriptor_EmbeddedCodexValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Equal(t, "codex", d.ID)
	require.Contains(t, d.Spawn.ForbidFlags, "exec")
	require.Contains(t, d.Spawn.Args, "--dangerously-bypass-hook-trust")

	// codex ships no --append-system-prompt, and its positional [PROMPT] IS the
	// user's first message — putting {context} there is what made codex answer
	// the handoff on sight. A FRESH codex session takes it silently through the
	// documented `developer_instructions` config key instead.
	require.Len(t, d.ContextInject, 1)
	require.Equal(t, "pass_arg", d.ContextInject[0].Verb)
	require.Equal(t, "-c", d.ContextInject[0].Args["arg"])
	require.Equal(t, "developer_instructions={context}", d.ContextInject[0].Args["value"])

	// A RESUMED codex session cannot be told anything through config (verified
	// against 0.139.0: `codex resume` rebuilds from a rollout that never records
	// developer instructions, `codex fork` behaves the same, and AGENTS.md is not
	// re-read), so the ONLY channel left is a USER MESSAGE — and what it carries is a
	// POINTER at the ledger already on disk, never the transcript, which would dump a
	// wall of handed-off text into the chat on every switch.
	require.Len(t, d.ResumeContextInject, 1)
	require.Equal(t, "pass_arg", d.ResumeContextInject[0].Verb)
	require.Equal(t, "{context_pointer}", d.ResumeContextInject[0].Args["positional"])
}

// mcpSpawnPlan renders providerID's whole spawn plan the way the daemon does, so
// every assertion below reads the argv a real CLI would actually receive rather
// than the descriptor's unexpanded source.
//
// Both shipped descriptors declare their MCP registration as a NAMED step group,
// which is the whole mechanism the per-provider tool-surface switch turns: a
// group can be dropped whole, and only a group can. codex is why — {runner_token}
// templates only one of its four MCP overrides, so any scheme that filtered
// templated steps would leave three behind and register a server with no
// arguments.
func TestDescriptors_DeclareTheirMCPRegistrationAsANamedGroup(t *testing.T) {
	claude, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Len(t, claude.MCPInject, 1)
	require.Equal(t, "--mcp-config", claude.MCPInject[0].Args["arg"])

	codex, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Len(t, codex.MCPInject, 4)
	for _, step := range codex.MCPInject {
		require.Equal(t, "-c", step.Args["arg"])
		require.Contains(t, step.Args["value"], "mcp_servers.crowbar.")
	}

	// Nothing MCP may be left in config_injection, or the switch would leave a
	// partial registration behind on exactly the provider it matters most for.
	for _, d := range []*agent.Descriptor{claude, codex} {
		for _, step := range d.ConfigInjection {
			for _, v := range step.Args {
				require.NotContains(t, asText(v), "mcp",
					"%s still registers MCP from config_injection, which the switch cannot reach", d.ID)
			}
		}
	}
}

func asText(v any) string {
	s, _ := v.(string)
	return s
}

// A third-party descriptor that declares no mcp_injection must LOAD and must
// simply register no tools. Requiring the field would make Crowbar's own MCP
// wiring a condition of being a provider at all.
func TestDescriptor_MCPInjectIsOptional(t *testing.T) {
	const minimal = `id: minimal
spawn:
  cmd: minimal
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`
	d, err := agent.LoadDescriptor([]byte(minimal))
	require.NoError(t, err)
	require.Empty(t, d.MCPInject)

	plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), Segid: "SEG", RunnerToken: "TOK",
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(plan.Cleanup)
	require.Empty(t, plan.Argv)
}

// The descriptor is handed over with its mcp_injection intact, which is the
// MCP-ENABLED case — the only one these assertions are about. The disabled case
// is asserted where the switch actually lives, in the usecase.
func mcpSpawnPlan(t *testing.T, providerID, repoID string) *agent.SpawnPlan {
	t.Helper()
	d, err := agent.ResolveDescriptor(t.TempDir(), providerID)
	require.NoError(t, err)
	plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(),
		CrowbarHook: "/usr/local/bin/crowbar",
		Segid:       "SEG", RunnerToken: "TOK", Provider: providerID,
		ProjectID: "P", RepoID: repoID, WorkspaceID: "W",
	}, nil, nil)
	require.NoError(t, err)
	t.Cleanup(plan.Cleanup)
	return plan
}

// wantMCPArgs is the argv `crowbar mcp` must be handed, element by element. Every
// scope value is its OWN element on purpose — see the empty-repo test below.
func wantMCPArgs(repoID string) []string {
	return []string{
		"mcp",
		"--segment", "SEG",
		"--token", "TOK",
		"--project", "P",
		"--workspace", "W",
		"--repo", repoID,
	}
}

func argAfter(t *testing.T, argv []string, flag string) string {
	t.Helper()
	i := indexOf(argv, flag)
	require.GreaterOrEqual(t, i, 0, "%s is not in the rendered argv", flag)
	require.Less(t, i+1, len(argv), "%s has no value", flag)
	return argv[i+1]
}

// configRHS returns the value side of the one `-c key=value` override whose key
// matches keyPrefix (which must include the `=`).
func configRHS(t *testing.T, argv []string, keyPrefix string) string {
	t.Helper()
	for i, a := range argv {
		if i > 0 && argv[i-1] == "-c" && strings.HasPrefix(a, keyPrefix) {
			return strings.TrimPrefix(a, keyPrefix)
		}
	}
	t.Fatalf("no -c override with key %q in %q", keyPrefix, argv)
	return ""
}

// Both descriptors must register the crowbar MCP server, and must do it through
// a channel that writes nothing into the user's home or repo — the same law that
// keeps Crowbar out of ~/.codex (see codex.yaml).
func TestDescriptors_RegisterTheCrowbarMCPServer(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		joined := strings.Join(mcpSpawnPlan(t, id, "R").Argv, " ")
		require.Contains(t, joined, "mcp", "%s does not register an MCP server", id)
		require.Contains(t, joined, "SEG")
		require.Contains(t, joined, "TOK")
	}
}

// claude takes its MCP registration as --mcp-config with a JSON STRING, so nothing
// is written to disk and the user's own MCP servers are untouched.
//
// The value is PARSED, never grepped: it is a JSON document embedded in a
// single-quoted YAML scalar that then becomes ONE argv token, and Expand is a plain
// text replacer with no escaping — a substring assertion would pass happily on a
// value whose quoting had come apart, which is the single most likely way this
// injection breaks.
func TestClaudeDescriptor_MCPConfigIsParseableJSON(t *testing.T) {
	var doc struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw := argAfter(t, mcpSpawnPlan(t, "claude", "R").Argv, "--mcp-config")
	require.NoError(t, json.Unmarshal([]byte(raw), &doc), "rendered --mcp-config is not valid JSON: %s", raw)

	server, ok := doc.MCPServers["crowbar"]
	require.True(t, ok, "no crowbar server in the rendered --mcp-config: %s", raw)
	require.Equal(t, "/usr/local/bin/crowbar", server.Command)
	require.Equal(t, wantMCPArgs("R"), server.Args)
}

// codex takes its MCP registration through the same session-layer -c channel the
// hooks use, so ~/.codex is untouched.
//
// codex parses a -c value as TOML. For the two shapes this descriptor emits — a
// basic string and an array of basic strings — the TOML and JSON encodings are
// byte-identical, so decoding them as JSON is a faithful well-formedness check
// without pulling a TOML parser into the module graph, and it catches the quoting
// mistakes a substring assertion cannot see.
func TestCodexDescriptor_MCPConfigValuesAreParseable(t *testing.T) {
	argv := mcpSpawnPlan(t, "codex", "R").Argv

	var command string
	rawCommand := configRHS(t, argv, "mcp_servers.crowbar.command=")
	require.NoError(t, json.Unmarshal([]byte(rawCommand), &command), "malformed command value: %s", rawCommand)
	require.Equal(t, "/usr/local/bin/crowbar", command)

	var args []string
	rawArgs := configRHS(t, argv, "mcp_servers.crowbar.args=")
	require.NoError(t, json.Unmarshal([]byte(rawArgs), &args), "malformed args value: %s", rawArgs)
	require.Equal(t, wantMCPArgs("R"), args)
}

// codex hands an MCP server a SCRUBBED environment — a fixed allowlist and nothing
// else — so the descriptor must name CROWBAR_HOME for forwarding or the relay dials
// the wrong daemon entirely (see the codex.yaml comment for the live probe and the
// 404 it produced). This asserts the forwarding LIST, not an injected value: a
// literal would turn an unset CROWBAR_HOME into a set one and send the relay to a
// home the daemon never used.
func TestCodexDescriptor_ForwardsCrowbarHomeToTheMCPServer(t *testing.T) {
	var forwarded []string
	raw := configRHS(t, mcpSpawnPlan(t, "codex", "R").Argv, "mcp_servers.crowbar.env_vars=")
	require.NoError(t, json.Unmarshal([]byte(raw), &forwarded), "malformed env_vars value: %s", raw)
	require.Equal(t, []string{"CROWBAR_HOME"}, forwarded)
}

// codex gates MCP tool calls behind a HUMAN approval modal, separately from the
// --ask-for-approval policy that governs the shell, and its default is to prompt. A
// Crowbar pane has nobody to answer that modal on the agent's behalf, so without an
// override the whole tool surface stalls on its first call (observed live on 0.146.0).
//
// The override is SERVER-WIDE by owner decision (2026-07-29): an agent that acts on
// real state without stopping for a per-call modal is what this surface is for, so
// every tool Crowbar registers is auto-approved by intent. It is also the only form
// that does not rot — the per-tool key it replaced needed a new line per tool, and an
// omitted line silently stalls that tool on the modal above forever.
//
// What the negative assertions still guard is SCOPE. The key must remain a field of
// crowbar's own server config: a global `mcp_servers.default_tools_approval_mode`
// (which codex rejects outright) or an `mcp_servers.tools.` form would reach the
// user's own MCP servers, which Crowbar has no business auto-approving.
func TestCodexDescriptor_AutoApprovesItsOwnServerWideAndNoOther(t *testing.T) {
	argv := mcpSpawnPlan(t, "codex", "R").Argv

	var mode string
	raw := configRHS(t, argv, "mcp_servers.crowbar.default_tools_approval_mode=")
	require.NoError(t, json.Unmarshal([]byte(raw), &mode), "malformed approval mode value: %s", raw)
	require.Equal(t, "approve", mode)

	for _, a := range argv {
		require.False(t, strings.HasPrefix(a, "mcp_servers.default_tools_approval_mode"),
			"the default must be scoped to crowbar's server; the global key is rejected by codex and would "+
				"reach every MCP server the user has configured")
	}
	require.NotContains(t, strings.Join(argv, " "), "mcp_servers.tools.",
		"the override must name crowbar's server, never every MCP server the user has configured")
}

// The empty-repo case is why scope travels as discrete array elements rather than
// through {scope_flags}: a project-home workspace has no repo id. Each element
// reaches `crowbar mcp` as its own argument, so the empty id arrives as an empty
// STRING instead of swallowing the next token the way a flat shell string would
// (see TemplateCtx.ScopeFlags for the bug that costs).
func TestDescriptors_MCPArgsSurviveAnEmptyRepoID(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		require.NotContains(t, strings.Join(mcpSpawnPlan(t, id, "").Argv, " "), "{repo_id}")
	}

	var doc struct {
		MCPServers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw := argAfter(t, mcpSpawnPlan(t, "claude", "").Argv, "--mcp-config")
	require.NoError(t, json.Unmarshal([]byte(raw), &doc), "an empty repo id broke the JSON: %s", raw)
	require.Equal(t, wantMCPArgs(""), doc.MCPServers["crowbar"].Args)

	var args []string
	rawArgs := configRHS(t, mcpSpawnPlan(t, "codex", "").Argv, "mcp_servers.crowbar.args=")
	require.NoError(t, json.Unmarshal([]byte(rawArgs), &args), "an empty repo id broke the array: %s", rawArgs)
	require.Equal(t, wantMCPArgs(""), args)
}

// claude's --mcp-config is VARIADIC (`--mcp-config <configs...>`): it keeps eating
// argv tokens until it meets a dash-prefixed one, so a bare positional after the
// JSON is silently taken as another config path. Observed on 2.1.220 — `claude
// --mcp-config '<json>' mcp list` dies with "MCP config file not found:
// /private/tmp/mcp".
//
// BuildSpawnPlan appends the session/handoff steps AFTER config_injection, and
// those can be positionals (a resumed session's id), so the descriptor has to
// guarantee a FLAG follows the JSON no matter what a caller adds. Here that is
// --settings, which is why mcp_injection renders ahead of config_injection and
// not behind it.
func TestClaudeDescriptor_MCPConfigIsNeverFollowedByAPositional(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	// A bare-positional pass_arg — the shape codex's resume_context_inject declares,
	// and the shape any extra step is free to take. claude's own resume steps happen to
	// lead with --resume, so this is the hazard's general form rather than a live path
	// today; the point is that the descriptor must not depend on that accident.
	extra := []agent.InjectStep{{Verb: "pass_arg", Args: map[string]any{"positional": "sid-native"}}}
	plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/usr/local/bin/crowbar",
		Segid: "SEG", RunnerToken: "TOK", Provider: "claude",
		ProjectID: "P", RepoID: "R", WorkspaceID: "W",
	}, nil, extra)
	require.NoError(t, err)
	t.Cleanup(plan.Cleanup)

	i := indexOf(plan.Argv, "--mcp-config")
	require.GreaterOrEqual(t, i, 0)
	require.Less(t, i+2, len(plan.Argv), "--mcp-config must not be the last flag: %q", plan.Argv)
	require.True(t, strings.HasPrefix(plan.Argv[i+2], "-"),
		"the variadic --mcp-config would swallow %q as another config path; argv was %q",
		plan.Argv[i+2], plan.Argv)
}

// The MCP registration must be config-only on BOTH providers. claude's
// --mcp-config takes the JSON inline precisely so there is no file whose lifetime
// anyone has to manage, and codex's -c overrides leave ~/.codex untouched — so the
// per-spawn tmp dir must hold exactly what it held before this existed: claude's
// hook settings.json and nothing else, codex's nothing at all.
func TestDescriptors_MCPRegistrationWritesNoFile(t *testing.T) {
	claude := mcpSpawnPlan(t, "claude", "R")
	entries, err := os.ReadDir(claude.TmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "settings.json", entries[0].Name())

	settings, err := os.ReadFile(argAfter(t, claude.Argv, "--settings"))
	require.NoError(t, err)
	require.NotContains(t, string(settings), "mcpServers",
		"the MCP server must ride --mcp-config, not the hook settings file")

	entries, err = os.ReadDir(mcpSpawnPlan(t, "codex", "R").TmpDir)
	require.NoError(t, err)
	require.Empty(t, entries, "the codex descriptor must write no files")
}

func TestLoadDescriptor_RejectsMissingID(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte("spawn:\n  cmd: x\n"))
	require.Error(t, err)
}

func TestParsePayload_JSON(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	m, err := d.ParsePayload([]byte(`{"session_id":"x"}`))
	require.NoError(t, err)
	require.Equal(t, "x", m["session_id"])
}

func TestParsePayload_UnknownFormatErrors(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`
id: p
spawn: { cmd: x, interactive_required: true }
hooks:
  format: toml
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: msg }
`))
	require.NoError(t, err)
	_, err = d.ParsePayload([]byte("x=1"))
	require.Error(t, err)
}

func TestLoadDescriptor_ParsesDisplayMetadata(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`id: demo
display_name: Demo Provider
icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M1 1h1v1H1z"/></svg>'
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`))
	require.NoError(t, err)
	require.Equal(t, "Demo Provider", d.DisplayName)
	require.Contains(t, d.Icon, "currentColor")
}

func TestValidate_DisplayFieldsAreOptional(t *testing.T) {
	// A descriptor with NO icon/display_name still validates: the display-only
	// carve-out must not break the "every engine field load-bearing" invariant.
	d, err := agent.LoadDescriptor([]byte(`id: bare
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`))
	require.NoError(t, err)
	require.Empty(t, d.Icon)
	require.Empty(t, d.DisplayName)
}

// TestResolveDescriptor_ProvidersOpenInAutoMode: a Crowbar chat is a working agent
// pane, so both CLIs open in their AUTO mode rather than the default
// ask-before-every-action one — being prompted per edit makes the pane useless.
// Neither uses the "skip every check" escape hatch (claude's bypassPermissions,
// codex's --dangerously-bypass-approvals-and-sandbox): codex still runs inside a
// workspace-write sandbox and escalates when it needs to leave it.
func TestResolveDescriptor_ProvidersOpenInAutoMode(t *testing.T) {
	claude, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Contains(t, claude.Spawn.Args, "--permission-mode")
	require.Contains(t, claude.Spawn.Args, "auto")
	require.NotContains(t, claude.Spawn.Args, "bypassPermissions")
	require.NotContains(t, claude.Spawn.Args, "--dangerously-skip-permissions")

	codex, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Contains(t, codex.Spawn.Args, "--sandbox")
	require.Contains(t, codex.Spawn.Args, "workspace-write")
	require.Contains(t, codex.Spawn.Args, "--ask-for-approval")
	require.Contains(t, codex.Spawn.Args, "on-request")
	require.NotContains(t, codex.Spawn.Args, "--dangerously-bypass-approvals-and-sandbox")
}
