package gitlab

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
	g := New(fakeCmd("main\ndevelop\n", 0))
	branches, err := g.ProtectedBranches(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop"}, branches)
}

func TestProtectedBranches_Error(t *testing.T) {
	dir := t.TempDir()
	g := New(fakeCmd("permission denied", 1))
	_, err := g.ProtectedBranches(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: protected-branches")
}

func TestRegression_PullRequestForBranch_MergedAfterRemoteBranchDeleted(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":3,"state":"merged","web_url":"https://gitlab.com/o/r/-/merge_requests/3","title":"Done","target_branch":"main","sha":"3d06bd4"}]`
	g := New(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0},
		{output: "", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 3, pr.Number)
	assert.Equal(t, "merged", pr.Status)
}

func TestPullRequestForBranch_IgnoresStaleMRAfterBranchNameReuse(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":3,"state":"merged","web_url":"https://gitlab.com/o/r/-/merge_requests/3","title":"Old","target_branch":"main","sha":"3d06bd4"}]`
	g := New(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0},
		{output: "", code: 1},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestPullRequestForBranch_OpenMR(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":7,"state":"opened","web_url":"https://gitlab.com/o/r/-/merge_requests/7","title":"My MR","target_branch":"main","sha":"deadbee"}]`
	g := New(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 7, pr.Number)
	assert.Equal(t, "open", pr.Status)
	assert.Equal(t, "main", pr.TargetBranch)
}

func TestPullRequestForBranch_MergedWithoutSHA(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":3,"state":"merged","web_url":"https://gitlab.com/o/r/-/merge_requests/3","title":"Done","target_branch":"main"}]`
	g := New(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, "merged", pr.Status)
}

func TestPullRequestForBranch_NoMRs(t *testing.T) {
	dir := t.TempDir()
	g := New(sequentialFake([]fakeResponse{
		{output: "[]", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestPullRequestForBranch_GlabError(t *testing.T) {
	dir := t.TempDir()
	g := New(sequentialFake([]fakeResponse{
		{output: "not authenticated", code: 1},
	}))
	_, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: list-mrs")
}

func TestMapState(t *testing.T) {
	assert.Equal(t, "open", mapState("opened"))
	assert.Equal(t, "merged", mapState("merged"))
	assert.Equal(t, "closed", mapState("closed"))
	assert.Equal(t, "closed", mapState("anything_else"))
}

func TestSelectBestMR_Empty(t *testing.T) {
	assert.Nil(t, selectBestMR(nil))
	assert.Nil(t, selectBestMR([]mrJSON{}))
}

func TestSelectBestMR_PreferOpen(t *testing.T) {
	mrs := []mrJSON{
		{IID: 1, State: "merged"},
		{IID: 2, State: "opened"},
	}
	best := selectBestMR(mrs)
	require.NotNil(t, best)
	assert.Equal(t, 2, best.IID)
}

func TestSelectBestMR_HigherIIDWins(t *testing.T) {
	mrs := []mrJSON{
		{IID: 5, State: "closed"},
		{IID: 3, State: "closed"},
		{IID: 8, State: "closed"},
	}
	best := selectBestMR(mrs)
	require.NotNil(t, best)
	assert.Equal(t, 8, best.IID)
}

func TestBetterMR_AGreaterIID(t *testing.T) {
	a := &mrJSON{IID: 10, State: "closed"}
	b := &mrJSON{IID: 5, State: "closed"}
	result := betterMR(a, b)
	assert.Equal(t, a, result)
}

func TestBetterMR_BothOpen_BHigher(t *testing.T) {
	a := &mrJSON{IID: 3, State: "opened"}
	b := &mrJSON{IID: 7, State: "opened"}
	result := betterMR(a, b)
	assert.Equal(t, b, result)
}

func TestParseMRList_InvalidJSON(t *testing.T) {
	_, err := parseMRList([]byte("not json"))
	require.Error(t, err)
}

func TestGlabProvider_OwnerAvatarURL(t *testing.T) {
	dir := t.TempDir()
	g := New(fakeCmd("https://gitlab.com/uploads/group/avatar/42/icon.png", 0))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/uploads/group/avatar/42/icon.png", got)
}

func TestGlabProvider_OwnerAvatarURL_CliError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := New(fakeCmd("", 1))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGlabProvider_OpenPullRequests(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[
		{"iid":7,"state":"opened","web_url":"https://gitlab.com/o/r/-/merge_requests/7","title":"Add A","source_branch":"feature/a","target_branch":"develop"},
		{"iid":8,"state":"opened","web_url":"https://gitlab.com/o/r/-/merge_requests/8","title":"Add B","source_branch":"feature/b","target_branch":"feature/a"}
	]`
	g := New(fakeCmd(mrJSON, 0))

	links, err := g.OpenPullRequests(context.Background(), dir)

	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, "feature/a", links[0].Head)
	assert.Equal(t, "develop", links[0].Base)
	assert.Equal(t, 7, links[0].Number)
	assert.Equal(t, "Add A", links[0].Title)
	assert.Equal(t, "feature/b", links[1].Head)
	assert.Equal(t, "feature/a", links[1].Base)
}

func TestGlabProvider_OpenPullRequests_NoOpenMRs(t *testing.T) {
	dir := t.TempDir()
	g := New(fakeCmd(`[]`, 0))

	links, err := g.OpenPullRequests(context.Background(), dir)

	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestGlabProvider_OpenPullRequests_GlabError(t *testing.T) {
	dir := t.TempDir()
	g := New(fakeCmd("not authenticated", 1))

	_, err := g.OpenPullRequests(context.Background(), dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: open-mrs")
}

func TestGlabProvider_OpenPullRequests_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	g := New(fakeCmd(`{not json`, 0))

	_, err := g.OpenPullRequests(context.Background(), dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: open-mrs: parse")
}

func TestProtectedBranches_AFailureReadsAsTheToolsOwnWords(t *testing.T) {
	g := New(fakeCmd("To get started with GitLab CLI, please run:  glab auth login", 1))
	_, err := g.ProtectedBranches(context.Background(), t.TempDir())

	require.Error(t, err)
	assert.Equal(t, "To get started with GitLab CLI, please run:  glab auth login", problem.Of(err).Message)
}
