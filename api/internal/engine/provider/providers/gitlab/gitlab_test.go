package gitlab

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

// fakeCmd returns a stub that prints output and exits with code.
// Production code sets cmd.Dir after creation; use t.TempDir() as the repoPath.
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
	g := NewWithExec(fakeCmd("main\ndevelop\n", 0))
	branches, err := g.ProtectedBranches(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "develop"}, branches)
}

func TestProtectedBranches_Error(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("permission denied", 1))
	_, err := g.ProtectedBranches(context.Background(), dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: protected-branches")
}

// TestRegression_PullRequestForBranch_MergedAfterRemoteBranchDeleted mirrors the
// GitHub regression: a merged MR whose source branch was deleted on the remote
// must still be observed. Gating the lookup on `git ls-remote` hid it, leaving the
// workspace pr-open forever.
func TestRegression_PullRequestForBranch_MergedAfterRemoteBranchDeleted(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":3,"state":"merged","web_url":"https://gitlab.com/o/r/-/merge_requests/3","title":"Done","target_branch":"main","sha":"3d06bd4"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0}, // glab mr list
		{output: "", code: 0},     // git merge-base --is-ancestor → branch contains the MR head
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 3, pr.Number)
	assert.Equal(t, "merged", pr.Status)
}

// TestPullRequestForBranch_IgnoresStaleMRAfterBranchNameReuse: glab matches MRs by
// source branch NAME, so fresh work reusing a merged branch's name matches the old
// MR. The new branch does not contain that MR's head commit, so it is not ours.
func TestPullRequestForBranch_IgnoresStaleMRAfterBranchNameReuse(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":3,"state":"merged","web_url":"https://gitlab.com/o/r/-/merge_requests/3","title":"Old","target_branch":"main","sha":"3d06bd4"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0}, // glab mr list
		{output: "", code: 1},     // git merge-base --is-ancestor → branch does NOT contain it
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

// TestPullRequestForBranch_OpenMR: an open MR still owns its source ref, so no
// containment check runs. The single queued response is the assertion —
// sequentialFake panics on an extra call.
func TestPullRequestForBranch_OpenMR(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":7,"state":"opened","web_url":"https://gitlab.com/o/r/-/merge_requests/7","title":"My MR","target_branch":"main","sha":"deadbee"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0}, // glab mr list, and nothing else
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 7, pr.Number)
	assert.Equal(t, "open", pr.Status)
	assert.Equal(t, "main", pr.TargetBranch)
}

// TestPullRequestForBranch_MergedWithoutSHA keeps the containment guard permissive
// when the provider gives us no head sha to check: a false negative here would
// resurrect the stale-status bug the guard is built alongside.
func TestPullRequestForBranch_MergedWithoutSHA(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[{"iid":3,"state":"merged","web_url":"https://gitlab.com/o/r/-/merge_requests/3","title":"Done","target_branch":"main"}]`
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: mrJSON, code: 0}, // glab mr list, and nothing else
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, "merged", pr.Status)
}

func TestPullRequestForBranch_NoMRs(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
		{output: "[]", code: 0},
	}))
	pr, err := g.PullRequestForBranch(context.Background(), dir, "my-branch")
	require.NoError(t, err)
	assert.Nil(t, pr)
}

func TestPullRequestForBranch_GlabError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(sequentialFake([]fakeResponse{
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
	g := NewWithExec(fakeCmd("https://gitlab.com/uploads/group/avatar/42/icon.png", 0))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/uploads/group/avatar/42/icon.png", got)
}

func TestGlabProvider_OwnerAvatarURL_CliError_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("", 1))
	got, err := g.OwnerAvatarURL(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestGlabProvider_OpenPullRequests maps every open MR to a source→target link in
// one glab call. Unlike PullRequestForBranch there is no ownership check: an OPEN
// MR still owns its source ref by name, so a stale-name match is impossible.
func TestGlabProvider_OpenPullRequests(t *testing.T) {
	dir := t.TempDir()
	mrJSON := `[
		{"iid":7,"state":"opened","web_url":"https://gitlab.com/o/r/-/merge_requests/7","title":"Add A","source_branch":"feature/a","target_branch":"develop"},
		{"iid":8,"state":"opened","web_url":"https://gitlab.com/o/r/-/merge_requests/8","title":"Add B","source_branch":"feature/b","target_branch":"feature/a"}
	]`
	g := NewWithExec(fakeCmd(mrJSON, 0))

	links, err := g.OpenPullRequests(context.Background(), dir)

	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, "feature/a", links[0].Head)
	assert.Equal(t, "develop", links[0].Base)
	assert.Equal(t, 7, links[0].Number)
	assert.Equal(t, "Add A", links[0].Title)
	// The chained MR is what makes this a graph rather than a flat list: b's base
	// is a's head, which is exactly the parent chain the import dialog walks.
	assert.Equal(t, "feature/b", links[1].Head)
	assert.Equal(t, "feature/a", links[1].Base)
}

func TestGlabProvider_OpenPullRequests_NoOpenMRs(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd(`[]`, 0))

	links, err := g.OpenPullRequests(context.Background(), dir)

	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestGlabProvider_OpenPullRequests_GlabError(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd("not authenticated", 1))

	_, err := g.OpenPullRequests(context.Background(), dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: open-mrs")
}

func TestGlabProvider_OpenPullRequests_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	g := NewWithExec(fakeCmd(`{not json`, 0))

	_, err := g.OpenPullRequests(context.Background(), dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab: open-mrs: parse")
}
