package github

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	output := args[len(args)-2]
	exitCode := args[len(args)-1]
	if exitCode != "0" {
		_, _ = fmt.Fprint(os.Stderr, output)
		os.Exit(1)
	}
	_, _ = fmt.Fprint(os.Stdout, output)
	os.Exit(0)
}

func fakeCmd(
	output string,
	exitCode int,
) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", output, fmt.Sprint(exitCode)}
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "GOCOVERDIR=" + flag.Lookup("test.gocoverdir").Value.String()}
		return cmd
	}
}

const testRemote = "https://github.com/owner/repo.git"

type fakeResponse struct {
	output string
	code   int
}

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
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "main\ndevelop\n", code: 0},
	}))
	branches, err := g.ProtectedBranches(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop"}, branches)
}

func TestProtectedBranches_UnparseableRemote(t *testing.T) {
	dir := t.TempDir()
	g := New("/srv/remotes/github.com/owner/repo.git", sequentialFake(nil))
	_, err := g.ProtectedBranches(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: protected-branches")
}

func TestProtectedBranches_GHError(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "authentication error", code: 1},
	}))
	_, err := g.ProtectedBranches(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: protected-branches")
}

func TestRegression_PullRequestForBranch_MergedAfterRemoteBranchDeleted(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":54,"state":"MERGED","url":"https://github.com/o/r/pull/54","title":"Done","baseRefName":"develop","headRefOid":"3d06bd4"}]`
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: prJSON, code: 0},
		{output: "", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 54, pr.Number)
	assert.Equal(t, "merged", pr.Status)
	assert.Equal(t, "develop", pr.TargetBranch)
}

func TestPullRequestForBranch_IgnoresStalePRAfterBranchNameReuse(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":54,"state":"MERGED","url":"https://github.com/o/r/pull/54","title":"Old","baseRefName":"develop","headRefOid":"3d06bd4"}]`
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: prJSON, code: 0},
		{output: "", code: 1},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestPullRequestForBranch_OpenPRSkipsContainmentCheck(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":42,"state":"OPEN","url":"https://github.com/o/r/pull/42","title":"My PR","baseRefName":"main","headRefOid":"deadbee"}]`
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: prJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	assert.Equal(t, "open", pr.Status)
	assert.Equal(t, "main", pr.TargetBranch)
}

func TestPullRequestForBranch_MergedWithoutHeadRefOid(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":5,"state":"MERGED","url":"https://github.com/o/r/pull/5","title":"Done","baseRefName":"main"}]`
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: prJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, "merged", pr.Status)
}

func TestPullRequestForBranch_PreferOpen(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[
		{"number":10,"state":"MERGED","url":"https://github.com/o/r/pull/10","title":"Old","baseRefName":"main"},
		{"number":20,"state":"OPEN","url":"https://github.com/o/r/pull/20","title":"New","baseRefName":"main"}
	]`
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: prJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 20, pr.Number)
	assert.Equal(t, "open", pr.Status)
}

func TestPullRequestForBranch_NoPRs(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "[]", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestOpenPullRequests_ParsesHeadBaseGraph(t *testing.T) {
	dir := t.TempDir()
	out := `[
		{"number":9324,"state":"OPEN","url":"u1","title":"t1","headRefName":"feat/9324","baseRefName":"feat/base"},
		{"number":10,"state":"OPEN","url":"u2","title":"t2","headRefName":"feat/base","baseRefName":"dev"}
	]`
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: out, code: 0},
	}))
	links, err := g.OpenPullRequests(context.Background(), dir)
	require.NoError(t, err)
	got := map[string]string{}
	for _, l := range links {
		got[l.Head] = l.Base
	}
	assert.Equal(t, "feat/base", got["feat/9324"])
	assert.Equal(t, "dev", got["feat/base"])
}

func TestOpenPullRequests_Empty(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "[]", code: 0},
	}))
	links, err := g.OpenPullRequests(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestPullRequestForBranch_GHError(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "not authenticated", code: 1},
	}))
	_, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: list-prs")
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
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "https://avatars.githubusercontent.com/u/123", code: 0},
	}))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/123", got)
}

func TestGHProvider_OwnerAvatarURL_CliError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "authentication error", code: 1},
	}))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestPullRequestForBranch_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "not json at all", code: 0},
	}))
	_, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: list-prs: parse")
}

func TestOpenPullRequests_GHError(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "not authenticated", code: 1},
	}))
	_, err := g.OpenPullRequests(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: open-prs")
}

func TestOpenPullRequests_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	g := New(testRemote, sequentialFake([]fakeResponse{
		{output: "not json at all", code: 0},
	}))
	_, err := g.OpenPullRequests(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: open-prs: parse")
}

func TestBetterPR_OpenBeatsClosedRegardlessOfNumber(t *testing.T) {
	a := &prJSON{Number: 1, State: "OPEN"}
	b := &prJSON{Number: 99, State: "CLOSED"}
	assert.Equal(t, a, betterPR(a, b), "the open PR must win even with a lower number")
}

func TestGHProvider_OwnerAvatarURL_SlugError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := New("totally-invalid", sequentialFake(nil))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestProtectedBranches_AFailureReadsAsTheToolsOwnWords(t *testing.T) {
	g := New(testRemote, fakeCmd("To get started with GitHub CLI, please run:  gh auth login", 1))
	_, err := g.ProtectedBranches(context.Background(), t.TempDir())

	require.Error(t, err)
	assert.Equal(t, "To get started with GitHub CLI, please run:  gh auth login", problem.Of(err).Message)
}
