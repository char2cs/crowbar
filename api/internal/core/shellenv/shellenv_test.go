package shellenv

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCmd returns an ExecFn producing a command that prints output and exits
// with code, ignoring the requested binary entirely.
func fakeCmd(output string, code int) ExecFn {
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		script := "printf '%s' " + shellQuote(output) + "; exit " + strconv.Itoa(code)
		return exec.CommandContext(ctx, "/bin/sh", "-c", script)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func getenvFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoginShellPath_UsesShellFromEnv(t *testing.T) {
	got, err := loginShellPath(
		context.Background(),
		fakeCmd("/opt/homebrew/bin:/usr/bin", 0),
		"darwin",
		getenvFrom(map[string]string{"SHELL": "/bin/zsh"}),
	)
	require.NoError(t, err)
	assert.Equal(t, "/opt/homebrew/bin:/usr/bin", got)
}

func TestLoginShellPath_ShellFailure(t *testing.T) {
	_, err := loginShellPath(
		context.Background(),
		fakeCmd("", 1),
		"linux",
		getenvFrom(map[string]string{"SHELL": "/bin/bash"}),
	)
	assert.Error(t, err)
}

func TestLoginShellPath_EmptyOutputIsError(t *testing.T) {
	_, err := loginShellPath(
		context.Background(),
		fakeCmd("   ", 0),
		"linux",
		getenvFrom(map[string]string{"SHELL": "/bin/bash"}),
	)
	assert.Error(t, err)
}

func TestResolveShell_FallsBackToDsclOnDarwin(t *testing.T) {
	// No SHELL in env; the injected exec answers the dscl query.
	shell := resolveShell(
		context.Background(),
		fakeCmd("UserShell: /bin/zsh\n", 0),
		"darwin",
		getenvFrom(nil),
	)
	assert.Equal(t, "/bin/zsh", shell)
}

func TestResolveShell_FallsBackToShWhenNothingResolves(t *testing.T) {
	assert.Equal(t, "/bin/sh", resolveShell(
		context.Background(),
		fakeCmd("", 1),
		"darwin",
		getenvFrom(nil),
	))
	assert.Equal(t, "/bin/sh", resolveShell(
		context.Background(),
		fakeCmd("irrelevant", 0),
		"linux",
		getenvFrom(nil),
	))
}

func TestDsclUserShell_UnparseableOutput(t *testing.T) {
	assert.Equal(t, "", dsclUserShell(context.Background(), fakeCmd("garbage", 0)))
}

func TestMergePaths_DedupesAndKeepsPrimaryOrder(t *testing.T) {
	got := mergePaths("/a:/b:/usr/bin", "/usr/bin:/c::/a")
	assert.Equal(t, "/a:/b:/usr/bin:/c", got)
}

func TestApplyLoginShellPath_MergesIntoProcessEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shells are POSIX only")
	}
	t.Setenv("SHELL", "/bin/sh") // real /bin/sh -l prints a real PATH
	t.Setenv("PATH", "/only-current-entry")

	got := ApplyLoginShellPath(context.Background())
	assert.Contains(t, got, "/only-current-entry", "current PATH entries must never be lost")
	assert.Equal(t, os.Getenv("PATH"), got)
}

func TestApplyLoginShellPath_KeepsPathOnFailure(t *testing.T) {
	t.Setenv("SHELL", "/nonexistent-shell-binary")
	t.Setenv("PATH", "/keep-me")

	got := ApplyLoginShellPath(context.Background())
	assert.Equal(t, "/keep-me", got)
	assert.Equal(t, "/keep-me", os.Getenv("PATH"))
}
