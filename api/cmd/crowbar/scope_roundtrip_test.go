package main

import (
	"bytes"
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
