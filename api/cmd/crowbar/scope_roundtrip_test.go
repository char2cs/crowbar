package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// This file guards the FULL in-PTY callback round-trip that the plain
// scopedAgentPath unit tests structurally cannot see:
//
//	descriptor hook template → engine/agent.Expand → flat shell string
//	  → /bin/sh word-splitting → argv → real cobra/pflag parse → scopedAgentPath
//
// Every earlier test entered that chain AFTER the shell, handing cobra a
// pre-split []string with an explicit "" repo — which is not what a shell can
// ever produce. It therefore missed a total outage of agent chats on
// project-home workspaces: those resolve an EMPTY RepoID, the old template
// `--repo {repo_id}` rendered a bare `--repo `, /bin/sh dropped the empty token,
// and pflag (which does not reject a dash-prefixed value) consumed the NEXT
// token as --repo's value. `crowbar hook session_start … --project P --repo
// --workspace W` parsed as repo="--workspace" plus a stray positional W, failing
// hook's ExactArgs(1) — so NO session_bound / turn_started / turn_stopped /
// title_set callback ever reached the daemon for a project-home chat.
//
// The tests below therefore go through a REAL /bin/sh and the REAL root command.

// ── The shell half ───────────────────────────────────────────────────────────

// argvThroughShell executes `command` with /bin/sh exactly as a vendor CLI runs a
// hook, with the crowbar binary replaced by a script that dumps its argv NUL-
// separated. The returned slice is precisely the argv the real crowbar binary
// would have parsed (its os.Args[1:]) — empty tokens collapsed, quotes honoured.
func argvThroughShell(t *testing.T, command string) []string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "argv")
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = append(os.Environ(), "CROWBAR_ARGV_OUT="+out)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoErrorf(t, cmd.Run(), "sh -c %q: %s", command, stderr.String())

	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	argv := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	require.NotEmpty(t, argv)
	return argv
}

// argvDumper writes the stand-in "crowbar" binary that argvThroughShell relies
// on: it records the argv the shell handed it and exits 0.
func argvDumper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crowbar")
	script := "#!/bin/sh\n: > \"$CROWBAR_ARGV_OUT\"\n" +
		"for a in \"$@\"; do printf '%s\\0' \"$a\" >> \"$CROWBAR_ARGV_OUT\"; done\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// ── The cobra half ───────────────────────────────────────────────────────────

type parsedCallback struct {
	project, repo, workspace string
	args                     []string
}

// parseThroughCobra feeds the shell-produced argv to the REAL root command —
// exactly as main() does with os.Args[1:] (the dumper records "$@", which likewise
// excludes the binary). Real subcommand resolution, real pflag parsing, real Args
// validator: ValidateArgs is what the pre-fix argv failed with cobra's
// "accepts 1 arg(s), received 2".
func parseThroughCobra(t *testing.T, argv []string) parsedCallback {
	t.Helper()
	target, rest, err := newRootCmd().Find(argv)
	require.NoError(t, err)
	require.NoError(t, target.ParseFlags(rest))

	args := target.Flags().Args()
	require.NoErrorf(t, target.ValidateArgs(args), "cobra rejected argv %q", argv)

	get := func(name string) string {
		v, err := target.Flags().GetString(name)
		require.NoError(t, err)
		return v
	}
	return parsedCallback{project: get("project"), repo: get("repo"), workspace: get("workspace"), args: args}
}

// ── Descriptor hook commands (the production template source) ────────────────

// hookCommands renders a provider's REAL embedded descriptor through the REAL
// BuildSpawnPlan and returns the hook commands it wrote, keyed by the vendor's
// event name. Reading them back off disk (rather than restating them in the test)
// is deliberate: it is the descriptor as the vendor CLI will actually read it, so
// reverting claude.yaml/codex.yaml to the broken flag triple fails these tests.
func hookCommands(t *testing.T, providerID string, ctx engineagent.TemplateCtx) map[string]string {
	t.Helper()
	d, err := engineagent.ResolveDescriptor("", providerID) // "" home → embedded default
	require.NoError(t, err)

	ctx.Tmp = t.TempDir()
	ctx.Cwd = t.TempDir()
	_, err = engineagent.BuildSpawnPlan(d, ctx, nil, nil)
	require.NoError(t, err)

	// The hook-config file's name is the descriptor's business (claude:
	// settings.json; codex: codex-home/hooks.json), so discover it by shape.
	commands := map[string]string{}
	require.NoError(t, filepath.WalkDir(ctx.Tmp, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		var file struct {
			Hooks map[string][]struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &file) != nil {
			return nil //nolint:nilerr // non-JSON siblings (codex auth.json/config.toml) are not hook configs
		}
		for event, matchers := range file.Hooks {
			for _, m := range matchers {
				for _, h := range m.Hooks {
					commands[event] = h.Command
				}
			}
		}
		return nil
	}))
	require.NotEmpty(t, commands, "descriptor %q wrote no hook config", providerID)
	return commands
}

// TestHookCallbackRoundTrip_ProjectHomeHasNoRepo is the regression guard for the
// project-home outage. For EVERY hook event of EVERY shipped provider, an empty
// RepoID must survive the shell and land as repo == "" — which routes the
// callback to the project-home agent mount instead of a 404 /repos//workspaces/…
func TestHookCallbackRoundTrip_ProjectHomeHasNoRepo(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			commands := hookCommands(t, provider, engineagent.TemplateCtx{
				CrowbarHook: argvDumper(t),
				Segid:       "SEG",
				Provider:    provider,
				ProjectID:   "PROJ",
				RepoID:      "", // ← project-home: WorktreeDir resolves no repo
				WorkspaceID: "WS",
			})
			require.Len(t, commands, 3, "expected session_start/user_prompt/turn_stop hooks")

			for event, command := range commands {
				got := parseThroughCobra(t, argvThroughShell(t, command))

				require.Equalf(t, "", got.repo,
					"%s hook: empty repo id must parse as empty, not swallow the next token", event)
				require.Equal(t, "PROJ", got.project)
				require.Equal(t, "WS", got.workspace)
				require.Lenf(t, got.args, 1, "%s hook: `hook <event>` takes exactly one positional", event)

				require.Equal(t, "/v0/projects/PROJ/home/agent/hooks",
					scopedAgentPath(got.project, got.repo, got.workspace, "/hooks"),
					"%s hook must target the project-home agent mount", event)
			}
		})
	}
}

// TestHookCallbackRoundTrip_WorkspaceScopedControl is the control: repo-home and
// worktree workspaces DO carry a repo id, and must keep the workspace-scoped path.
func TestHookCallbackRoundTrip_WorkspaceScopedControl(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			commands := hookCommands(t, provider, engineagent.TemplateCtx{
				CrowbarHook: argvDumper(t),
				Segid:       "SEG",
				Provider:    provider,
				ProjectID:   "PROJ",
				RepoID:      "REPO",
				WorkspaceID: "WS",
			})
			for event, command := range commands {
				got := parseThroughCobra(t, argvThroughShell(t, command))

				require.Equal(t, "REPO", got.repo)
				require.Lenf(t, got.args, 1, "%s hook: `hook <event>` takes exactly one positional", event)
				require.Equal(t, "/v0/projects/PROJ/repos/REPO/workspaces/WS/agent/hooks",
					scopedAgentPath(got.project, got.repo, got.workspace, "/hooks"),
					"%s hook must stay workspace-scoped when a repo id exists", event)
			}
		})
	}
}

// TestChatRenameRoundTrip_ProjectHomeHasNoRepo covers the agent auto-title
// callback. Its command line lives in core/config's title_instruction (the LLM
// retypes it verbatim), so it goes through the same shell + pflag chain — and,
// unlike the hooks, it carries positionals AFTER the flags (chatid + title),
// which is exactly what a swallowed --repo value shifts out of alignment.
func TestChatRenameRoundTrip_ProjectHomeHasNoRepo(t *testing.T) {
	ctx := engineagent.TemplateCtx{
		CrowbarHook: argvDumper(t),
		Chatid:      "chat-1",
		ProjectID:   "PROJ",
		RepoID:      "",
		WorkspaceID: "WS",
	}
	// The instruction is prose wrapping one command line; the agent runs that line.
	command := renameCommandLine(t, engineagent.Expand(titleInstruction(t), ctx))

	got := parseThroughCobra(t, argvThroughShell(t, command))

	require.Equal(t, "", got.repo)
	require.Equal(t, []string{"chat-1", "<title>"}, got.args, "chat rename takes exactly chatid + title")
	require.Equal(t, "/v0/projects/PROJ/home/agent/chats/chat-1/rename?source=agent",
		scopedAgentPath(got.project, got.repo, got.workspace, "/chats/"+got.args[0]+"/rename?source=agent"))
}

// TestHandoffDumpRoundTrip_ProjectHomeHasNoRepo covers the third callback. It has
// no descriptor template (it is invoked by hand / by the agent), so it is driven
// through the same shell + cobra chain from the same {scope_flags} rendering.
func TestHandoffDumpRoundTrip_ProjectHomeHasNoRepo(t *testing.T) {
	ctx := engineagent.TemplateCtx{
		CrowbarHook: argvDumper(t),
		Chatid:      "chat-1",
		ProjectID:   "PROJ",
		RepoID:      "",
		WorkspaceID: "WS",
	}
	command := engineagent.Expand("{crowbar} handoff dump {scope_flags} {chatid}", ctx)

	got := parseThroughCobra(t, argvThroughShell(t, command))

	require.Equal(t, "", got.repo)
	require.Equal(t, []string{"chat-1"}, got.args)
	require.Equal(t, "/v0/projects/PROJ/home/agent/chats/chat-1/handoff",
		scopedAgentPath(got.project, got.repo, got.workspace, "/chats/"+got.args[0]+"/handoff"))
}

// titleInstruction returns the shipped default title_instruction prompt through
// the SAME accessor production uses, rather than restating it — so a regression in
// core/config's default.yaml fails here. CROWBAR_HOME is pointed at an empty temp
// dir so the developer's own ~/.crowbar/config.yaml cannot overlay the default.
func titleInstruction(t *testing.T) string {
	t.Helper()
	t.Setenv(metadata.HomeEnvVar, t.TempDir())
	instruction := config.GetPrompts().TitleInstruction
	require.NotEmpty(t, instruction)
	return instruction
}

// renameCommandLine plucks the single `chat rename …` command line out of the
// title instruction's surrounding prose — the line the agent is told to run.
func renameCommandLine(t *testing.T, instruction string) string {
	t.Helper()
	for _, line := range strings.Split(instruction, "\n") {
		if strings.Contains(line, "chat rename") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("no `chat rename` line in title instruction:\n%s", instruction)
	return ""
}
