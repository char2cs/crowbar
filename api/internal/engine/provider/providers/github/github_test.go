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

// TestRegression_PullRequestForBranch_MergedAfterRemoteBranchDeleted pins the
// production bug: GitHub deletes the head ref when a PR merges, so gating the
// lookup on `git ls-remote origin refs/heads/<branch>` bailed out before ever
// running gh — the workspace stayed pr-open forever. gh still reports the merged
// PR for a deleted head ref, and the lookup must read it.
func TestRegression_PullRequestForBranch_MergedAfterRemoteBranchDeleted(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":54,"state":"MERGED","url":"https://github.com/o/r/pull/54","title":"Done","baseRefName":"develop","headRefOid":"3d06bd4"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: prJSON, code: 0}, // gh pr list
		{output: "", code: 0},     // git merge-base --is-ancestor → branch contains the PR head
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 54, pr.Number)
	assert.Equal(t, "merged", pr.Status)
	assert.Equal(t, "develop", pr.TargetBranch)
}

// TestPullRequestForBranch_IgnoresStalePRAfterBranchNameReuse covers the flip
// side of dropping the ls-remote gate: gh matches PRs by head branch NAME alone,
// so fresh work on a branch whose name was used by an already-merged PR matches
// that stale PR. The new branch does not contain the PR's head commit, so the PR
// is not this branch's and must be ignored.
func TestPullRequestForBranch_IgnoresStalePRAfterBranchNameReuse(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":54,"state":"MERGED","url":"https://github.com/o/r/pull/54","title":"Old","baseRefName":"develop","headRefOid":"3d06bd4"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: prJSON, code: 0}, // gh pr list
		{output: "", code: 1},     // git merge-base --is-ancestor → branch does NOT contain it
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

// TestPullRequestForBranch_OpenPRSkipsContainmentCheck: an open PR still owns its
// head ref, so a name match is authoritative and no git call is needed. The single
// queued response is the assertion — sequentialFake panics on an extra call.
func TestPullRequestForBranch_OpenPRSkipsContainmentCheck(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":42,"state":"OPEN","url":"https://github.com/o/r/pull/42","title":"My PR","baseRefName":"main","headRefOid":"deadbee"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: prJSON, code: 0}, // gh pr list, and nothing else
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	assert.Equal(t, "open", pr.Status)
	assert.Equal(t, "main", pr.TargetBranch)
}

// TestPullRequestForBranch_MergedWithoutHeadRefOid keeps the containment guard
// permissive when the provider gives us no head sha to check: reporting the PR is
// the pre-guard behaviour, and a false negative here would resurrect the very
// stale-status bug the guard is built alongside.
func TestPullRequestForBranch_MergedWithoutHeadRefOid(t *testing.T) {
	dir := t.TempDir()
	prJSON := `[{"number":5,"state":"MERGED","url":"https://github.com/o/r/pull/5","title":"Done","baseRefName":"main"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: prJSON, code: 0}, // gh pr list, and nothing else
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
	g := NewWithExec(sequentialFake([]fakeResponse{
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
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "[]", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

// TestOpenPullRequests_ParsesHeadBaseGraph pins the repo-wide open-PR graph used
// by import auto-parenting: one gh call yields every open PR's head→base edge.
func TestOpenPullRequests_ParsesHeadBaseGraph(t *testing.T) {
	dir := t.TempDir()
	out := `[
		{"number":9324,"state":"OPEN","url":"u1","title":"t1","headRefName":"feat/9324","baseRefName":"feat/base"},
		{"number":10,"state":"OPEN","url":"u2","title":"t2","headRefName":"feat/base","baseRefName":"dev"}
	]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: out, code: 0}, // gh pr list --state open, and nothing else
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
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "[]", code: 0},
	}))
	links, err := g.OpenPullRequests(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestPullRequestForBranch_GHError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
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
		{output: "https://github.com/owner/repo.git", code: 0},           // git remote get-url
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

// TestPullRequestForBranch_MalformedJSON covers parsePRList's error branch as
// reached from PullRequestForBranch: gh returning something that is not a JSON
// array must surface as a wrapped "list-prs: parse" error, not panic or
// silently report no PR.
func TestPullRequestForBranch_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "not json at all", code: 0},
	}))
	_, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: list-prs: parse")
}

// TestOpenPullRequests_GHError covers OpenPullRequests' runGH error branch: a
// failing `gh pr list` must surface as a wrapped "open-prs" error.
func TestOpenPullRequests_GHError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "not authenticated", code: 1},
	}))
	_, err := g.OpenPullRequests(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: open-prs")
}

// TestOpenPullRequests_MalformedJSON covers OpenPullRequests' parsePRList
// error branch, distinct from PullRequestForBranch's — the two callers wrap the
// same parse failure with different messages ("open-prs: parse" vs
// "list-prs: parse"), so each needs its own proof.
func TestOpenPullRequests_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "not json at all", code: 0},
	}))
	_, err := g.OpenPullRequests(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github: open-prs: parse")
}

// TestBetterPR_OpenBeatsClosedRegardlessOfNumber covers betterPR's
// aOpen-and-not-bOpen branch specifically: an open PR must win even when the
// closed one has a higher number, which TestBetterPR_AGreaterOrEqualNumber
// (both closed) and TestBetterPR_BothOpen_BHigher (both open) do not exercise.
func TestBetterPR_OpenBeatsClosedRegardlessOfNumber(t *testing.T) {
	a := &prJSON{Number: 1, State: "OPEN"}
	b := &prJSON{Number: 99, State: "CLOSED"}
	assert.Equal(t, a, betterPR(a, b), "the open PR must win even with a lower number")
}

// TestWithWaitDelay_SetsWaitDelayOnConstructedCmd covers the wrapper New()
// actually uses in production (NewWithExec's fakes bypass it entirely): every
// *exec.Cmd it builds must carry WaitDelay, so a killed subprocess whose pipes
// are held open by a grandchild still releases instead of leaking.
func TestWithWaitDelay_SetsWaitDelayOnConstructedCmd(t *testing.T) {
	var gotName string
	inner := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		return exec.CommandContext(ctx, name, args...)
	}

	wrapped := withWaitDelay(inner)
	cmd := wrapped(context.Background(), "echo", "hi")

	assert.Equal(t, "echo", gotName, "the inner execFn must still be invoked with the same args")
	assert.Equal(t, waitDelay, cmd.WaitDelay, "every constructed Cmd must carry the wait delay")
}

func TestGHProvider_OwnerAvatarURL_SlugError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("", 1)) // git remote fails
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err) // soft failure
	assert.Empty(t, got)
}
