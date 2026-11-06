package agents_test

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestStartServe_SpeaksOnTheChildsStdinAndStdout(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `read line; echo "got $line"`)
	stdio, err := agents.StartServe(cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })

	_, err = stdio.In.Write([]byte("hello\n"))
	require.NoError(t, err)
	line, err := bufio.NewReader(stdio.Out).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "got hello\n", line)
	require.NoError(t, cmd.Wait())
}

// Serve's exit is EOF on its stdout only if nothing else holds the write
// end; a copy kept here would leave a dead serve looking alive.
func TestStartServe_AServeThatExitsIsEOF(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "exit 0")
	stdio, err := agents.StartServe(cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })
	require.NoError(t, cmd.Wait())

	out, ok := stdio.Out.(*os.File)
	require.True(t, ok)
	require.NoError(t, out.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = out.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

func TestStartServe_AServeThatCannotStartIsAnError(t *testing.T) {
	_, err := agents.StartServe(exec.CommandContext(t.Context(), "/nonexistent/serve"))

	require.Error(t, err)
}

func TestAgent_StartAPIConnRefusesAHooksTransportDescriptor(t *testing.T) {
	in, out, _ := testutil.StdioPipe(t)
	_, err := get(t, "claude").StartAPIConn(t.Context(), agents.APIStdio{In: in, Out: out}, nil)
	require.ErrorIs(t, err, agents.ErrAPITransportNotDeclared)
	_, err = in.Write([]byte("{}\n"))
	assert.ErrorIs(t, err, os.ErrClosed, "a refused start closes the stdio it was handed")
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
`+v3EventsBlock+apiRuntime+`
    serve:  [acme, app-server, --cwd, "{cwd}"]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize, notify: initialized }
`)
	a, err := agents.New().Get(context.Background(), home, "api-transport")
	require.NoError(t, err)

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{Cwd: "/work"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server", "--cwd", "/work"}, serveArgv)

	attachArgv, ok := a.APIAttachArgv(agents.TemplateCtx{Session: "sess-1"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "resume", "sess-1"}, attachArgv)
}

// TestAgent_APIAttachArgvCarriesConfigInjection pins the sibling gap: the
// attached process is an ORDINARY hooks-transport CLI from the instant it
// starts, and config_injection is where session_start/user_prompt/turn_stop/
// etc. all get wired to the hook server — without this, the one process a
// non-hotswap provider hands a live turn over to would report nothing back
// to Crowbar's ledger at all.
func TestAgent_APIAttachArgvCarriesConfigInjection(t *testing.T) {
	home := t.TempDir()
	writeDescriptor(t, home, "api-attach-hooks", `
id: api-attach-hooks
spawn:
  cmd: acme
  interactive_required: true
`+v3EventsBlock+apiRuntime+`
    serve:  [acme, app-server]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize, notify: initialized }
config_injection:
  - pass_arg: { arg: "-c", value: 'hooks.Stop=[{hooks=[{type="command",command="{hook_url}/turn_stop"}]}]' }
`)
	a, err := agents.New().Get(context.Background(), home, "api-attach-hooks")
	require.NoError(t, err)

	attachArgv, ok := a.APIAttachArgv(agents.TemplateCtx{
		Session: "sess-1", Segid: "seg-1", HookBase: "http://h", ProjectID: "p", Provider: "acme",
	})
	require.True(t, ok)
	assert.Equal(t, []string{
		"acme", "resume", "sess-1",
		"-c", `hooks.Stop=[{hooks=[{type="command",command="http://h/v0/projects/p/home/chats/runners/seg-1/hooks/acme/turn_stop"}]}]`,
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
`+v3EventsBlock+apiRuntime+`
    serve:  [acme, app-server]
    attach: [acme, resume, "{session_id}"]
    handshake: { call: initialize, notify: initialized }
`)
	a, err := agents.New().Get(context.Background(), home, "api-attach-spawn-args")
	require.NoError(t, err)

	attachArgv, ok := a.APIAttachArgv(agents.TemplateCtx{Session: "sess-1"})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "--trust-workaround", "resume", "sess-1"}, attachArgv,
		"spawn.args must land right after the executable, before the attach subcommand's own args")

	// serve is a genuinely different subcommand (app-server, not an
	// interactive TUI) — spawn.args are interactive-only flags and must NOT
	// leak onto it.
	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server"}, serveArgv,
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
`+v3EventsBlock+apiRuntime+`
    serve:  [acme, app-server]
    handshake: { call: initialize, notify: initialized }
mcp_injection:
  - pass_arg: { arg: "-c", value: 'mcp_servers.crowbar.command="{crowbar}"' }
  - pass_arg: { arg: "-c", value: 'mcp_servers.crowbar.args=["mcp","--segment","{segid}"]' }
`)
	a, err := agents.New().Get(context.Background(), home, "api-mcp")
	require.NoError(t, err)

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{
		Segid: "seg-1", CrowbarBin: "/bin/crowbar",
	})
	require.True(t, ok)
	assert.Equal(t, []string{
		"acme", "app-server",
		"-c", `mcp_servers.crowbar.command="/bin/crowbar"`,
		"-c", `mcp_servers.crowbar.args=["mcp","--segment","seg-1"]`,
	}, serveArgv, "the serve process must carry the SAME crowbar MCP registration a hooks-attached CLI gets")
}

// argv is world-readable through ps: the relay reads the token from a 0600 file.
func TestShippedAgents_KeepTheRunnerTokenOffArgv(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			tmp := t.TempDir()
			ctx := agents.TemplateCtx{
				Tmp: tmp, Cwd: t.TempDir(), Segid: "SEG", RunnerToken: "SECRET-RUNNER-TOKEN",
				ProjectID: "P", RepoID: "R", WorkspaceID: "W", CrowbarBin: "/bin/crowbar", HookBase: "http://127.0.0.1:4242",
			}
			plan, err := get(t, id).SpawnPlan(ctx, nil, nil)
			require.NoError(t, err)

			argv := strings.Join(plan.Argv, " ")
			assert.NotContains(t, argv, "SECRET-RUNNER-TOKEN")
			assert.Contains(t, argv, `"--token-file","`+filepath.Join(tmp, "mcp.token")+`"`)
			raw, err := os.ReadFile(filepath.Join(tmp, "mcp.token"))
			require.NoError(t, err)
			assert.Equal(t, "SECRET-RUNNER-TOKEN", strings.TrimSpace(string(raw)))
			info, err := os.Stat(filepath.Join(tmp, "mcp.token"))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		})
	}
}

func TestAgent_Codex_ServeProcessReportsOverOneChannel(t *testing.T) {
	a := get(t, "codex")
	ctx := agents.TemplateCtx{Cwd: `/work/tree "a"`, CrowbarBin: "/bin/crowbar", Tmp: t.TempDir()}

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
`+v3EventsBlock+apiRuntime+`
    serve:  [acme, app-server]
    handshake: { call: initialize, notify: initialized }
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
		Model: "deep", Effort: "high",
	})
	require.True(t, ok)
	assert.Equal(t, []string{
		"acme", "app-server",
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
`+v3EventsBlock+apiRuntime+`
    serve:  [acme, app-server]
    handshake: { call: initialize, notify: initialized }
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

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server"}, serveArgv)
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
		Model: "gpt-5.4-codex", Effort: "high", Tmp: t.TempDir(),
	})

	require.True(t, ok)
	assert.Contains(t, serveArgv, `model="gpt-5.4-codex"`,
		"an adopted codex connection has no argv but this one; a model that misses it is not run")
	assert.Contains(t, serveArgv, `model_reasoning_effort="high"`)
}
