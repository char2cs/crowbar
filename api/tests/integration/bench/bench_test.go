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

var wsPushSeq atomic.Int64

var mergeSeq atomic.Int64

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

	env.Close(t)
}
