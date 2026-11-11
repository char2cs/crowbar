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

// wsPushSeq is a package-level monotonic counter, used to give each push
// iteration a distinguishable PR URL so its own row can be picked out of the
// stream.
var wsPushSeq atomic.Int64

// mergeSeq is a package-level monotonic counter for unique worktree branches.
var mergeSeq atomic.Int64

// runWorkspaceWSPushIteration performs one benchmark iteration: pushes a
// provider-state change for wsID (the mock-provider seam, PushProviderState)
// and waits for its workspace row on the projects scope — the
// state-change→publish hot path. One workspace is reused, each push with a new
// PR URL, so the timing is push latency rather than worktree provisioning.
func runWorkspaceWSPushIteration(
	t *testing.T,
	env *kit.Env,
	wsID string,
	watcher *kit.EntityWatcher,
) {
	t.Helper()
	seq := wsPushSeq.Add(1)
	prURL := fmt.Sprintf("https://bench.test/pr/%d", seq)
	env.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    prURL,
		PRTitle:  "bench",
	})
	kit.WaitForWorkspace(t, watcher, wsID, 10*time.Second, func(m map[string]any) bool {
		return m["prUrl"] == prURL
	})
}

// TestBenchmarkWorkspaceWSPushLatency measures the end-to-end push latency from
// a workspace state change to its row reaching a projects-scope holder.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorkspaceWSPushLatency(t *testing.T) {
	const n = 50
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "bench-ws", "")
	wsID, _ := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/bench-ws-push", "")

	watcher := env.HoldProjects(t)

	result := kit.RunBenchmark(t, "WorkspaceWSPushLatency", n, func() {
		runWorkspaceWSPushIteration(t, env, wsID, watcher)
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

	childID, childChatID := env.CreateWorkspaceWithChat(
		t,
		imported.ProjectID,
		imported.RepoID,
		branch,
		imported.WorkspaceID,
	)
	childPath := env.WorktreePath(imported.ProjectID, imported.RepoID, childID)

	kit.CommitFile(t, childPath, "bench.txt", fmt.Sprintf("bench content %d\n", seq), "bench commit")
	// The child's fork point is the parent's tip until the merge moves it, so
	// only a fork point past this one says the merge has landed.
	before := kit.RevParse(t, imported.RepoPath, "HEAD")

	watcher := env.HoldProjects(t)
	mergeResp := env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+childChatID+"/merge-into-parent",
		map[string]any{"strategy": "merge"})
	kit.RequireStatus(t, mergeResp, http.StatusAccepted)
	mergeResp.Body.Close()
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		fp, _ := m["forkPointSha"].(string)
		return fp != "" && fp != before && fp == kit.RevParse(t, imported.RepoPath, "HEAD")
	})
}

// TestBenchmarkWorktreeMergeIntoParent measures the local child→parent merge hot path.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorktreeMergeIntoParent(t *testing.T) {
	const n = 10

	kit.RequireNoLeakedProcesses(t)
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "bench-merge", "")

	result := kit.RunBenchmark(t, "WorktreeMergeIntoParent", n, func() {
		runWorktreeMergeIteration(t, env, imported)
	})

	t.Logf("WorktreeMergeIntoParent p50=%v p99=%v", result.P50, result.P99)
	kit.AssertNoRegression(t, result)

	// Each merge's tail (summary resync, conflict prediction) is still running
	// git in the repo; teardown must end it before the repo is removed.
	env.Close(t)
}
