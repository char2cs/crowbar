package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// This file guards the FULL in-PTY callback round-trip that the plain
// scopedAgentPath unit tests structurally cannot see:
//
//	descriptor hook template → engine/agents.Expand → flat shell string
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
	project, repo, workspace, segment string
	args                              []string
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
	// segment is only bound on commands that carry a runner id (hook, mcp);
	// handoff dump has no such flag, so it is read optionally.
	segment := ""
	if target.Flags().Lookup("segment") != nil {
		segment = get("segment")
	}
	return parsedCallback{
		project: get("project"), repo: get("repo"), workspace: get("workspace"),
		segment: segment, args: args,
	}
}

// ── Descriptor hook commands (the production template source) ────────────────

// hookCommands renders a provider's REAL embedded descriptor through the REAL
// BuildSpawnPlan and returns the hook commands it wrote, keyed by the vendor's
// event name. Reading them back off disk (rather than restating them in the test)
// is deliberate: it is the descriptor as the vendor CLI will actually read it, so
// reverting claude.yaml/codex.yaml to the broken flag triple fails these tests.
func hookCommands(t *testing.T, providerID string, ctx engineagents.TemplateCtx) map[string]string {
	t.Helper()
	d, err := engineagents.New().Get(context.Background(), "", providerID) // "" home → embedded default
	require.NoError(t, err)

	root := t.TempDir()
	ctx.Tmp = root
	ctx.Cwd = t.TempDir()
	plan, err := d.SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)

	// A descriptor may deliver its hook commands EITHER as a written config file
	// (claude: --settings settings.json) or as config overrides on the argv (codex:
	// -c hooks.SessionStart=..., so that Crowbar never has to own codex's home).
	// Collect from both, keyed by the vendor's event name.
	commands := map[string]string{}
	for _, a := range plan.Argv {
		for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
			if !strings.HasPrefix(a, "hooks."+ev+"=") {
				continue
			}
			if i := strings.Index(a, `command="`); i >= 0 {
				rest := a[i+len(`command="`):]
				if j := strings.Index(rest, `"`); j >= 0 {
					commands[ev] = rest[:j]
				}
			}
		}
	}

	// ...plus any hook-config FILE the descriptor wrote (claude's settings.json).
	require.NoError(t, filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
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
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if json.Unmarshal(data, &file) != nil {
			return nil // not a hook config
		}
		for event, groups := range file.Hooks {
			for _, g := range groups {
				for _, h := range g.Hooks {
					if h.Command != "" {
						commands[event] = h.Command
					}
				}
			}
		}
		return nil
	}))

	return commands
}

// TestHookCallbackRoundTrip_ProjectHomeHasNoRepo is the regression guard for the
// project-home outage. For EVERY hook event of EVERY shipped provider, an empty
// RepoID must survive the shell and land as repo == "" — which routes the
// callback to the project-home agent mount instead of a 404 /repos//chats/…
func TestHookCallbackRoundTrip_ProjectHomeHasNoRepo(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			commands := hookCommands(t, provider, engineagents.TemplateCtx{
				CrowbarHook: argvDumper(t),
				Segid:       "SEG",
				Provider:    provider,
				ProjectID:   "PROJ",
				RepoID:      "", // ← project-home: WorktreeDir resolves no repo
				WorkspaceID: "WS",
			})
			// Every registered channel, not a fixed count: the guard is that no hook
			// a descriptor adds can quietly skip this round trip, and asserting a
			// number would only mean someone updated the number.
			require.NotEmpty(t, commands)
			require.Contains(t, commands, "SessionStart")
			require.Contains(t, commands, "UserPromptSubmit")
			require.Contains(t, commands, "Stop")

			for event, command := range commands {
				got := parseThroughCobra(t, argvThroughShell(t, command))

				require.Equalf(t, "", got.repo,
					"%s hook: empty repo id must parse as empty, not swallow the next token", event)
				require.Equal(t, "PROJ", got.project)
				require.Equal(t, "WS", got.workspace)
				require.Lenf(t, got.args, 1, "%s hook: `hook <event>` takes exactly one positional", event)

				require.Equal(t, "/v0/projects/PROJ/home/chats/hooks",
					scopedAgentPath(got.project, got.repo, got.workspace, "/hooks"),
					"%s hook must target the project-home agent mount", event)
			}
		})
	}
}

// TestHookCallbackRoundTrip_WorkspaceScopedControl is the control: repo-home and
// worktree workspaces DO carry a repo id, and must keep the repo-scoped path
// (Task 17: no workspace segment, regardless of which workspace it came from).
func TestHookCallbackRoundTrip_WorkspaceScopedControl(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			commands := hookCommands(t, provider, engineagents.TemplateCtx{
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
				require.Equal(t, "/v0/projects/PROJ/repos/REPO/chats/hooks",
					scopedAgentPath(got.project, got.repo, got.workspace, "/hooks"),
					"%s hook must stay repo-scoped when a repo id exists", event)
			}
		})
	}
}

// TestHookRoundTrip_ScopeFlagsMidLine_ProjectHomeHasNoRepo keeps the empty-repo
// guard alive at the one position the shipped descriptor templates do not reach:
// {scope_flags} in the MIDDLE of the command line, with a flag AND a positional
// still to come after it.
//
// The descriptor hook templates above render {scope_flags} last, so a swallowed
// --repo value there runs off the end of the argv. Mid-line it eats a REAL token and
// shifts everything after it — `--repo --segment SEG` parses as repo="--segment"
// with SEG left as a stray positional, blowing `hook`'s ExactArgs(1). That is the
// exact shape the retired title instruction had (it put the scope flags before
// --segment and the title), and it is the shape any future callback template can
// take again, so the guard outlives the command it was written against: `hook`
// carries the identical --segment + positional plumbing.
func TestHookRoundTrip_ScopeFlagsMidLine_ProjectHomeHasNoRepo(t *testing.T) {
	ctx := engineagents.TemplateCtx{
		CrowbarHook: argvDumper(t),
		Segid:       "SEG-1",
		Provider:    "claude",
		ProjectID:   "PROJ",
		RepoID:      "", // ← project-home: WorktreeDir resolves no repo
		WorkspaceID: "WS",
	}
	command := engineagents.Expand(
		"{crowbar} hook {scope_flags} --segment {segid} --provider {provider} session_start", ctx,
	)

	got := parseThroughCobra(t, argvThroughShell(t, command))

	require.Equal(t, "", got.repo,
		"an empty repo id rendered mid-line must parse as empty, not swallow the following flag")
	require.Equal(t, "SEG-1", got.segment)
	require.Equal(t, []string{"session_start"}, got.args,
		"the positional after the scope flags must stay the event name")
	require.Equal(t, "/v0/projects/PROJ/home/chats/hooks",
		scopedAgentPath(got.project, got.repo, got.workspace, "/hooks"))
}

// TestHandoffDumpRoundTrip_ProjectHomeHasNoRepo covers the third callback. It has
// no descriptor template (it is invoked by hand / by the agent), so it is driven
// through the same shell + cobra chain from the same {scope_flags} rendering.
// handoff dump takes a REAL chat id typed at debug time — never a template
// token baked in at spawn — so it is written directly into the command line.
func TestHandoffDumpRoundTrip_ProjectHomeHasNoRepo(t *testing.T) {
	ctx := engineagents.TemplateCtx{
		CrowbarHook: argvDumper(t),
		ProjectID:   "PROJ",
		RepoID:      "",
		WorkspaceID: "WS",
	}
	command := engineagents.Expand("{crowbar} handoff dump {scope_flags} chat-1", ctx)

	got := parseThroughCobra(t, argvThroughShell(t, command))

	require.Equal(t, "", got.repo)
	require.Equal(t, []string{"chat-1"}, got.args)
	require.Equal(t, "/v0/projects/PROJ/home/chats/chat-1/handoff",
		scopedAgentPath(got.project, got.repo, got.workspace, "/"+got.args[0]+"/handoff"))
}
