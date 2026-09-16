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
// iteration a distinguishable PR URL so its own WS frame can be picked out of
// the stream.
var wsPushSeq atomic.Int64

// mergeSeq is a package-level monotonic counter for unique worktree branches.
var mergeSeq atomic.Int64

// runWorkspaceWSPushIteration performs one benchmark iteration: pushes a
// provider-state change for wsID (the mock-provider seam, PushProviderState)
// and waits for the corresponding worktree_state frame on watcher. This is the
// state-change→broadcast hot path.
//
// It no longer creates a fresh workspace per iteration (spec §8 step 6 deleted
// POST .../workspaces, and the atomic chat-scoped create that replaced it does
// its git worktree provisioning SYNCHRONOUSLY — the create response no longer
// races a background broadcast the way the old 202 create did, so timing it
// would measure request handling, not push latency). Reusing one workspace and
// varying the pushed PR URL isolates the same thing the old benchmark cared
// about: how fast a workspace state change reaches a connected WS client,
// through the identical hub-broadcast → chat-feed fan-out path
// (container.go's pushChatWorktree) production's provider poll rides.
func runWorkspaceWSPushIteration(
	t *testing.T,
	env *kit.Env,
	wsID string,
	watcher *kit.WSWatcher,
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
	watcher.ReadUntil(t, 10*time.Second, func(raw map[string]any) bool {
		return kit.WorktreeFrame(raw)["prUrl"] == prURL
	})
}

// TestBenchmarkWorkspaceWSPushLatency measures the end-to-end push latency from
// a workspace state change to the WS event being received by a connected chat
// client.
//
// This is a latency regression test, not a standard Go benchmark. Run with
// -tags=integration and UPDATE_BASELINE=1 to record a new baseline.
func TestBenchmarkWorkspaceWSPushLatency(t *testing.T) {
	const n = 50
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "bench-ws", "")
	wsID, chatID := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/bench-ws-push", "")

	watcher := env.DialChat(t, chatID)

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

	watcher := env.DialChat(t, childChatID)
	mergeResp := env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+childChatID+"/merge-into-parent",
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
