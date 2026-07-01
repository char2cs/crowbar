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

// runWorkspaceWSPushIteration performs one benchmark iteration: posts a 202
// workspace create and waits for the corresponding WS broadcast (status:"new")
// on a shared repo-scoped watcher. This is the create→broadcast hot path.
func runWorkspaceWSPushIteration(
	t *testing.T,
	env *kit.Env,
	imported kit.ImportedRepo,
	watcher *kit.WSWatcher,
) {
	t.Helper()
	seq := wsPushSeq.Add(1)
	safeName := strings.ReplaceAll(t.Name(), "/", "-")
	branch := fmt.Sprintf("feature/bench-ws-%s-%d", safeName, seq)

	resp := env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces",
		map[string]any{"branch": branch})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	watcher.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		return m["branch"] == branch && m["status"] == "new"
	})
}

// TestBenchmarkWorkspaceWSPushLatency measures the end-to-end push latency from a
// 202 Workspace Create to the WS event being received by a connected client.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorkspaceWSPushLatency(t *testing.T) {
	const n = 50
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "bench-ws", "")

	watcher := env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)

	result := kit.RunBenchmark(t, "WorkspaceWSPushLatency", n, func() {
		runWorkspaceWSPushIteration(t, env, imported, watcher)
	})

	t.Logf("WorkspaceWSPushLatency p50=%v p99=%v", result.P50, result.P99)
	kit.AssertNoRegression(t, result)
}

// runWorktreeMergeIteration performs one benchmark iteration: creates a child
// worktree, commits a file, and merges it back into the adopted-main parent.
func runWorktreeMergeIteration(
	t *testing.T,
	env *kit.Env,
	imported kit.ImportedRepo,
) {
	t.Helper()
	seq := mergeSeq.Add(1)
	safeName := strings.ReplaceAll(t.Name(), "/", "-")
	branch := fmt.Sprintf("feature/bench-%s-%d", safeName, seq)

	childID := env.CreateChildWorkspace(
		t,
		imported.ProjectID,
		imported.RepoID,
		branch,
		imported.WorkspaceID,
	)
	childPath := env.WorktreePath(imported.ProjectID, imported.RepoID, childID)

	kit.CommitFile(t, childPath, "bench.txt", fmt.Sprintf("bench content %d\n", seq), "bench commit")

	watcher := env.DialWorkspace(t, imported.ProjectID, imported.RepoID, childID)
	mergeResp := env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces/"+childID+"/merge-into-parent",
		map[string]any{"strategy": "merge"})
	kit.RequireStatus(t, mergeResp, http.StatusAccepted)
	mergeResp.Body.Close()
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		fp, _ := m["forkPointSha"].(string)
		return fp != "" && fp == kit.RevParse(t, imported.RepoPath, "HEAD")
	})
}

// TestBenchmarkWorktreeMergeIntoParent measures the local child→parent merge hot path.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorktreeMergeIntoParent(t *testing.T) {
	const n = 10

	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "bench-merge", "")

	result := kit.RunBenchmark(t, "WorktreeMergeIntoParent", n, func() {
		runWorktreeMergeIteration(t, env, imported)
	})

	t.Logf("WorktreeMergeIntoParent p50=%v p99=%v", result.P50, result.P99)
	kit.AssertNoRegression(t, result)
}
