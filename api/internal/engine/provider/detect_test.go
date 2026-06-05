package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is the subprocess entry point used by detectFakeCmd.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	fmt.Fprint(os.Stdout, args[len(args)-2])
	if args[len(args)-1] != "0" {
		os.Exit(1)
	}
	os.Exit(0)
}

// detectFakeCmd creates a cmd that prints output and exits with code.
func detectFakeCmd(
	output string,
	exitCode int,
) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		exitStr := "0"
		if exitCode != 0 {
			exitStr = fmt.Sprintf("%d", exitCode)
		}
		cs := []string{"-test.run=TestHelperProcess", "--", output, exitStr}
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

type detectFakeResponse struct {
	output string
	code   int
}

// detectSequentialFake sequences detectFakeCmd responses, one per call.
func detectSequentialFake(
	responses []detectFakeResponse,
) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	i := 0
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		if i >= len(responses) {
			panic(fmt.Sprintf("detectSequentialFake: unexpected extra call %d", i+1))
		}
		resp := responses[i]
		i++
		return detectFakeCmd(resp.output, resp.code)(ctx, name, args...)
	}
}

func TestDetect_GitHub(t *testing.T) {
	dir := t.TempDir()
	execFn := detectSequentialFake([]detectFakeResponse{
		{output: "https://github.com/owner/repo.git", code: 0},
		{output: "", code: 0},
	})

	res, err := DetectWithExec(context.Background(), dir, execFn)
	require.NoError(t, err)
	assert.Equal(t, "github", res.Kind)
	assert.True(t, res.Enabled)
}

func TestDetect_GitLab(t *testing.T) {
	dir := t.TempDir()
	execFn := detectSequentialFake([]detectFakeResponse{
		{output: "https://gitlab.com/owner/repo.git", code: 0},
		{output: "", code: 0},
	})

	res, err := DetectWithExec(context.Background(), dir, execFn)
	require.NoError(t, err)
	assert.Equal(t, "gitlab", res.Kind)
	assert.True(t, res.Enabled)
}

func TestDetect_Unknown(t *testing.T) {
	dir := t.TempDir()
	execFn := detectFakeCmd("https://bitbucket.org/owner/repo.git", 0)

	res, err := DetectWithExec(context.Background(), dir, execFn)
	require.NoError(t, err)
	assert.Equal(t, "none", res.Kind)
	assert.False(t, res.Enabled)
}

func TestDetect_GitError(t *testing.T) {
	dir := t.TempDir()
	execFn := detectFakeCmd("", 1)

	res, err := DetectWithExec(context.Background(), dir, execFn)
	require.NoError(t, err) // graceful degradation
	assert.Equal(t, "none", res.Kind)
	assert.False(t, res.Enabled)
}

func TestDetect_CLINotAuthed(t *testing.T) {
	dir := t.TempDir()
	execFn := detectSequentialFake([]detectFakeResponse{
		{output: "https://github.com/owner/repo.git", code: 0},
		{output: "not logged in", code: 1},
	})

	res, err := DetectWithExec(context.Background(), dir, execFn)
	require.NoError(t, err)
	assert.Equal(t, "github", res.Kind)
	assert.False(t, res.Enabled)
}

func TestKindFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/a/b", "github"},
		{"git@github.com:a/b.git", "github"},
		{"https://gitlab.com/a/b", "gitlab"},
		{"https://gitlab.mycompany.com/a/b", "gitlab"},
		{"https://bitbucket.org/a/b", "none"},
		{"", "none"},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			assert.Equal(t, c.want, kindFromURL(c.url))
		})
	}
}

func TestDetect_DegradedInNonGitDir(t *testing.T) {
	res, err := Detect(context.Background(), t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "none", res.Kind)
	assert.False(t, res.Enabled)
}

func TestFallbackProtectedBranches(t *testing.T) {
	branches := FallbackProtectedBranches()
	assert.Equal(t, []string{"main", "develop", "master"}, branches)

	// Ensure it returns a copy, not the original slice.
	branches[0] = "mutated"
	assert.Equal(t, "main", DefaultProtectedBranches[0])
}
