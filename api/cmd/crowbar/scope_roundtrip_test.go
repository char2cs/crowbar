package main

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/home"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

func argvDumper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crowbar")
	script := "#!/bin/sh\n: > \"$CROWBAR_ARGV_OUT\"\n" +
		"for a in \"$@\"; do printf '%s\\0' \"$a\" >> \"$CROWBAR_ARGV_OUT\"; done\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

type parsedCallback struct {
	project, repo, workspace, segment string
	args                              []string
}

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
	segment := ""
	if target.Flags().Lookup("segment") != nil {
		segment = get("segment")
	}
	return parsedCallback{
		project: get("project"), repo: get("repo"), workspace: get("workspace"),
		segment: segment, args: args,
	}
}

func TestMCPRoundTrip_ScopeFlagsMidLine_ProjectHomeHasNoRepo(t *testing.T) {
	ctx := engineagents.TemplateCtx{
		CrowbarBin:  argvDumper(t),
		Segid:       "SEG-1",
		ProjectID:   "PROJ",
		RepoID:      "",
		WorkspaceID: "WS",
	}
	command := engineagents.Expand("{crowbar} mcp {scope_flags} --segment {segid} --token-file T", ctx)

	got := parseThroughCobra(t, argvThroughShell(t, command))

	require.Equal(t, "", got.repo,
		"an empty repo id rendered mid-line must parse as empty, not swallow the following flag")
	require.Equal(t, "SEG-1", got.segment)
	require.Empty(t, got.args, "nothing after the scope flags may be left a stray positional")
	require.Equal(t, "/v0/projects/PROJ/home/chats/runners/SEG-1/mcp",
		scopedAgentPath(got.project, got.repo, got.workspace, "/runners/"+got.segment+"/mcp"))
}

func TestHandoffDumpRoundTrip_ProjectHomeHasNoRepo(t *testing.T) {
	ctx := engineagents.TemplateCtx{
		CrowbarBin:  argvDumper(t),
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

func TestHandoffDumpRoundTrip_DialsTheSocketTheDaemonBound_ForANonCleanHome(t *testing.T) {
	base, err := os.MkdirTemp("", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	t.Setenv(home.EnvVar, base+"/x/")

	ln, err := transports.NewSocket("unix://")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	reached := make(chan string, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached <- r.URL.Path
		if _, err := w.Write([]byte(`{"success":true,"data":{"handoff":""}}`)); err != nil {
			t.Error(err)
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	h, err := home.Resolve()
	require.NoError(t, err)
	ctx := engineagents.TemplateCtx{
		CrowbarBin: argvDumper(t), ProjectID: "PROJ", RepoID: "REPO", WorkspaceID: "WS",
		CrowbarHome: h.Root(), DaemonHost: "unix://" + ln.Addr().String(),
	}
	command := engineagents.Expand("{crowbar} handoff dump {scope_flags} chat-1", ctx)

	root := newRootCmd()
	root.SetArgs(argvThroughShell(t, command))
	require.NoError(t, root.Execute())

	select {
	case path := <-reached:
		require.Equal(t, "/v0/projects/PROJ/repos/REPO/chats/chat-1/handoff", path)
	default:
		t.Fatalf("the callback never reached the socket the daemon bound (%s)", ln.Addr())
	}
}
