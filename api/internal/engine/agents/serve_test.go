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

	serveArgv, ok := a.APIServeArgv(agents.TemplateCtx{})
	require.True(t, ok)
	assert.Equal(t, []string{"acme", "app-server"}, serveArgv,
		"spawn.args are interactive-TUI-only and must not appear on the serve argv")
}

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

func TestRegression_ACodexChatsChosenModelReachesItsServeProcess(t *testing.T) {
	serveArgv, ok := get(t, "codex").APIServeArgv(agents.TemplateCtx{
		Model: "gpt-5.4-codex", Effort: "high", Tmp: t.TempDir(),
	})

	require.True(t, ok)
	assert.Contains(t, serveArgv, `model="gpt-5.4-codex"`,
		"an adopted codex connection has no argv but this one; a model that misses it is not run")
	assert.Contains(t, serveArgv, `model_reasoning_effort="high"`)
}

func TestRegression_ACodexTerminalViewKeepsTheChatsSelection(t *testing.T) {
	attachArgv, ok := get(t, "codex").APIAttachArgv(agents.TemplateCtx{
		Session: "sess-1", Model: "gpt-5.4-codex", Effort: "low", PermissionLevel: "trusted",
		Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarBin: "/bin/crowbar", CrowbarHome: t.TempDir(),
		Segid: "seg-1", RunnerToken: "tok", HookBase: "http://h", Provider: "codex", ProjectID: "p",
	})

	require.True(t, ok)
	assert.Contains(t, strings.Join(attachArgv, " "), "--model gpt-5.4-codex")
	assert.Contains(t, attachArgv, `model_reasoning_effort="low"`)
	assert.Contains(t, strings.Join(attachArgv, " "), "--sandbox workspace-write")
	assert.Contains(t, strings.Join(attachArgv, " "), "--ask-for-approval on-request")
}
