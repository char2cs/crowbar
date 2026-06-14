package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is the subprocess used by fakeCmd.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	output := args[len(args)-2]
	exitCode := args[len(args)-1]
	fmt.Fprint(os.Stdout, output)
	if exitCode != "0" {
		os.Exit(1)
	}
	os.Exit(0)
}

// fakeCmd returns an execCommand stub that prints output and exits with code.
// Production code sets cmd.Dir after creation; the dir must exist.
// Use t.TempDir() as the repoPath in tests.
func fakeCmd(
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

type fakeResponse struct {
	output string
	code   int
}

// sequentialFake sequences multiple fake commands, one per call.
// It panics if called more times than there are responses.
func sequentialFake(
	responses []fakeResponse,
) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	i := 0
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		if i >= len(responses) {
			panic(fmt.Sprintf("sequentialFake: unexpected extra call %d", i+1))
		}
		resp := responses[i]
		i++
		return fakeCmd(resp.output, resp.code)(ctx, name, args...)
	}
}

func TestProtectedBranches_Success(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "https://github.com/owner/repo.git", code: 0}, // git remote get-url
		{output: "main\ndevelop\n", code: 0},                   // gh api
	}))
	branches, err := g.ProtectedBranches(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop"}, branches)
}

func TestProtectedBranches_RemoteError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("", 1))
	_, err := g.ProtectedBranches(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: protected-branches")
}

func TestProtectedBranches_GHError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "https://github.com/owner/repo.git", code: 0},
		{output: "authentication error", code: 1},
	}))
	_, err := g.ProtectedBranches(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: protected-branches")
}

func TestPullRequestForBranch_NoUpstream(t *testing.T) {
	dir := t.TempDir()
	// ls-remote returns empty output → no upstream
	g := NewWithExec(fakeCmd("", 0))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestPullRequestForBranch_OpenPR(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":42,"state":"OPEN","url":"https://github.com/o/r/pull/42","title":"My PR","baseRefName":"main"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "abc123\trefs/heads/my-branch", code: 0}, // ls-remote → has upstream
		{output: prJSON, code: 0},                         // gh pr list
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	assert.Equal(t, "open", pr.Status)
	assert.Equal(t, "main", pr.TargetBranch)
}

func TestPullRequestForBranch_PreferOpen(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[
		{"number":10,"state":"MERGED","url":"https://github.com/o/r/pull/10","title":"Old","baseRefName":"main"},
		{"number":20,"state":"OPEN","url":"https://github.com/o/r/pull/20","title":"New","baseRefName":"main"}
	]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "abc123\trefs/heads/my-branch", code: 0},
		{output: prJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 20, pr.Number)
	assert.Equal(t, "open", pr.Status)
}

func TestPullRequestForBranch_MergedState(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":5,"state":"MERGED","url":"https://github.com/o/r/pull/5","title":"Done","baseRefName":"main"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "abc123\trefs/heads/my-branch", code: 0},
		{output: prJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, "merged", pr.Status)
}

func TestPullRequestForBranch_NoPRs(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "abc123\trefs/heads/my-branch", code: 0},
		{output: "[]", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestPullRequestForBranch_GHError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "abc123\trefs/heads/my-branch", code: 0},
		{output: "not authenticated", code: 1},
	}))
	_, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: list-prs")
}

func TestPullRequestForBranch_LSRemoteError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("", 1))
	_, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: pr-for-branch")
}

func TestParsePRList_InvalidJSON(t *testing.T) {
	_, err := parsePRList([]byte("not json"))
	require.Error(t, err)
}

func TestMapState(t *testing.T) {
	assert.Equal(t, "open", mapState("OPEN"))
	assert.Equal(t, "open", mapState("open"))
	assert.Equal(t, "merged", mapState("MERGED"))
	assert.Equal(t, "merged", mapState("merged"))
	assert.Equal(t, "closed", mapState("CLOSED"))
	assert.Equal(t, "closed", mapState("anything_else"))
}

func TestSelectBestPR_Empty(t *testing.T) {
	assert.Nil(t, selectBestPR(nil))
	assert.Nil(t, selectBestPR([]prJSON{}))
}

func TestSelectBestPR_HigherNumberWins(t *testing.T) {
	prs := []prJSON{
		{Number: 1, State: "CLOSED"},
		{Number: 5, State: "CLOSED"},
		{Number: 3, State: "CLOSED"},
	}
	best := selectBestPR(prs)
	require.NotNil(t, best)
	assert.Equal(t, 5, best.Number)
}

func TestBetterPR_AGreaterOrEqualNumber(t *testing.T) {
	a := &prJSON{Number: 10, State: "CLOSED"}
	b := &prJSON{Number: 5, State: "CLOSED"}
	// a.Number > b.Number → a wins
	result := betterPR(a, b)
	assert.Equal(t, a, result)
}

func TestBetterPR_BothOpen_BHigher(t *testing.T) {
	a := &prJSON{Number: 5, State: "OPEN"}
	b := &prJSON{Number: 10, State: "OPEN"}
	result := betterPR(a, b)
	assert.Equal(t, b, result)
}

func TestParseLines(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"main\ndevelop\n", []string{"main", "develop"}},
		{"", []string{}},
		{"  main  \n\n  develop  ", []string{"main", "develop"}},
	}
	for _, c := range cases {
		got := parseLines(c.input)
		assert.Equal(t, c.want, got)
	}
}

func TestGHProvider_OwnerAvatarURL(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "https://github.com/owner/repo.git", code: 0}, // git remote get-url
		{output: "https://avatars.githubusercontent.com/u/123", code: 0}, // gh api
	}))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/123", got)
}

func TestGHProvider_OwnerAvatarURL_CliError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "https://github.com/owner/repo.git", code: 0},
		{output: "authentication error", code: 1}, // gh api fails
	}))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err) // soft failure
	assert.Empty(t, got)
}

func TestGHProvider_OwnerAvatarURL_SlugError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("", 1)) // git remote fails
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err) // soft failure
	assert.Empty(t, got)
}
