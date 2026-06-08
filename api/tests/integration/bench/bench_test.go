//go:build integration

package bench_test

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// wsPushSeq is a package-level monotonic counter for unique workspace branches.
var wsPushSeq atomic.Int64

// mergeSeq is a package-level monotonic counter for unique worktree branches.
var mergeSeq atomic.Int64

// runWorkspaceWSPushIteration performs one benchmark iteration: creates a
// workspace and waits for the corresponding WS broadcast event.
func runWorkspaceWSPushIteration(
	t *testing.T,
	env *kit.Env,
	watcher *kit.WSWatcher,
) {
	t.Helper()
	seq := wsPushSeq.Add(1)
	safeName := strings.ReplaceAll(
		t.Name(),
		"/",
		"-",
	)
	branch := fmt.Sprintf(
		"feature/bench-ws-%s-%d",
		safeName,
		seq,
	)
	resp := env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": branch,
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	wsID := kit.MutationID(t, resp)
	kit.WaitForWorkspace(
		t,
		watcher,
		wsID,
		10*time.Second,
		func(_ map[string]any) bool { return true },
	)
}

// TestBenchmarkWorkspaceWSPushLatency measures the end-to-end push latency from a
// Workspace Create (SendWait) to the WS event being received by a connected client.
// This is the critical broadcast hot path for the Workspaces topic.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorkspaceWSPushLatency(t *testing.T) {
	const n = 50
	env := kit.BuildEnv(t)

	// Register the repo once; workspace creates reference it by ID.
	repoResp := env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "bench-p",
		"name":      "repo",
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	watcher := env.DialWorkspaces(t, "?projectId=bench-p")

	result := kit.RunBenchmark(
		t,
		"WorkspaceWSPushLatency",
		n,
		func() {
			runWorkspaceWSPushIteration(t, env, watcher)
		},
	)

	t.Logf(
		"WorkspaceWSPushLatency p50=%v p99=%v",
		result.P50,
		result.P99,
	)
	kit.AssertNoRegression(
		t,
		result,
	)
}

// runWorktreeMergeIteration performs one benchmark iteration: creates a parent
// workspace, creates a child worktree, commits a file, and merges it back.
func runWorktreeMergeIteration(
	t *testing.T,
	env *kit.Env,
	repoPath string,
	baseBranch string,
) {
	t.Helper()
	seq := mergeSeq.Add(1)
	safeName := strings.ReplaceAll(
		t.Name(),
		"/",
		"-",
	)
	branch := fmt.Sprintf(
		"feature/bench-%s-%d",
		safeName,
		seq,
	)

	// Create parent workspace via HTTP; capture the server-assigned UUID.
	parentResp := env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": baseBranch,
	})
	kit.RequireStatus(t, parentResp, http.StatusCreated)
	parentID := kit.MutationID(t, parentResp)

	// Create child workspace via POST /v0/workspaces with parentId.
	childResp := env.POST(t, "/v0/workspaces", map[string]any{
		"repoId":   "r1",
		"branch":   branch,
		"parentId": parentID,
	})
	kit.RequireStatus(t, childResp, http.StatusCreated)
	childID := kit.MutationID(t, childResp)

	// Fetch child detail to get the worktree path.
	getResp := env.GET(t, "/v0/workspaces/"+childID)
	kit.RequireStatus(t, getResp, http.StatusOK)
	var childWs map[string]any
	kit.DecodeEnvData(t, getResp, &childWs)
	childPath := childWs["worktreePath"].(string)

	kit.CommitFile(
		t,
		childPath,
		"bench.txt",
		fmt.Sprintf("bench content %d\n", seq),
		"bench commit",
	)

	// Merge child into parent via HTTP.
	mergeResp := env.POST(t, "/v0/workspaces/"+childID+"/merge-into-parent", map[string]any{
		"strategy": "merge",
	})
	kit.RequireStatus(t, mergeResp, http.StatusOK)
	mergeResp.Body.Close()
}

// TestBenchmarkWorktreeMergeIntoParent measures the local child→parent merge hot path.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorktreeMergeIntoParent(t *testing.T) {
	const n = 10

	// Build the environment once outside the loop to avoid spinning up n
	// server stacks (HTTP listeners, SQLite files, engine goroutines) simultaneously.
	env := kit.BuildEnv(t)
	repoPath := kit.InitRepo(t)
	kit.GitRun(t, repoPath, "branch", "-m", "main", "feature/bench-base")
	baseBranch := kit.BranchName(
		t,
		repoPath,
	)

	// Insert the repository record once via HTTP; it is shared across all benchmark iterations.
	repoResp := env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	result := kit.RunBenchmark(
		t,
		"WorktreeMergeIntoParent",
		n,
		func() {
			runWorktreeMergeIteration(
				t,
				env,
				repoPath,
				baseBranch,
			)
		},
	)

	t.Logf(
		"WorktreeMergeIntoParent p50=%v p99=%v",
		result.P50,
		result.P99,
	)
	kit.AssertNoRegression(
		t,
		result,
	)
}
