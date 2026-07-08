//go:build integration

package lifecycle_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test entry point; delegates to kit.Main.
func TestMain(m *testing.M) {
	kit.Main(m)
}

// LifecycleSuite verifies end-to-end lifecycle flows through the full HTTP+WS+SQLite stack.
type LifecycleSuite struct {
	kit.IntegrationSuite
}

// TestLifecycleSuite runs the LifecycleSuite via testify.
func TestLifecycleSuite(t *testing.T) {
	suite.Run(
		t,
		new(LifecycleSuite),
	)
}

// TestLifecycle_WorkspaceCreate202ThenWS proves the full hierarchical create
// path: POST .../workspaces → 202 → projection → hub.BroadcastWorkspace → WS
// client (spec §4/§5). The created workspace first arrives as status:"new" and
// then transitions to its ready (status-absent / omitempty) terminal state.
func (s *LifecycleSuite) TestLifecycle_WorkspaceCreate202ThenWS() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "lifecycle", "")

	watcher := s.Env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)
	resp := s.Env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces",
		map[string]any{"branch": "feature/lifecycle"})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	created := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["branch"] == "feature/lifecycle" && m["status"] == "new"
	})
	wsID, _ := created["id"].(string)
	s.Require().NotEmpty(wsID)
	s.Assert().Equal("feature/lifecycle", created["branch"])
	s.Assert().Equal(imported.RepoID, created["repoId"])
	s.Assert().Equal(imported.ProjectID, created["projectId"])
}

// TestLifecycle_SyncClearsLastError proves a workspace edit → stage → commit →
// sync flow round-trips: the workspace stays status "new" after committing (per
// D4 — status is not cleared by HasCommits) and carries no lastError.
func (s *LifecycleSuite) TestLifecycle_SyncClearsLastError() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "lifecycle-sync", "")
	wsID := s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/lifecycle-sync")
	base := "/v0/projects/" + imported.ProjectID + "/repos/" + imported.RepoID + "/workspaces/" + wsID

	saveResp := s.Env.PUT(t, base+"/files/content", map[string]any{
		"path":    "a.txt",
		"content": "aaa\n",
	})
	kit.RequireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	stageResp := s.Env.POST(t, base+"/git/stage", map[string]any{
		"paths": []string{"a.txt"},
	})
	kit.RequireStatus(t, stageResp, http.StatusOK)
	stageResp.Body.Close()

	commitResp := s.Env.POST(t, base+"/git/commit", map[string]any{
		"subject": "add sync test file",
		"author":  "Test <t@t.com>",
	})
	kit.RequireStatus(t, commitResp, http.StatusOK)
	commitResp.Body.Close()

	watcher := s.Env.DialWorkspace(t, imported.ProjectID, imported.RepoID, wsID)
	syncResp := s.Env.POST(t, base+"/sync", nil)
	kit.RequireStatus(t, syncResp, http.StatusAccepted)
	syncResp.Body.Close()

	// Per D4 status STAYS "new" after sync with commits; the workspace must carry
	// no lastError. Wait on the terminal "new" frame and assert lastError empty.
	msg := kit.WaitForWorkspaceState(t, watcher, wsID, "new", 5*time.Second)
	le, _ := msg["lastError"].(string)
	s.Assert().Empty(le, "sync must not set a lastError")
}

// TestLifecycle_MergeEligibilityTrueWhenParentIdle proves that a child whose
// parent is idle (not locked/deleted) carries canMergeLocally:true and the
// parent's branch in the WS DTO (spec §10).
func (s *LifecycleSuite) TestLifecycle_MergeEligibilityTrueWhenParentIdle() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "merge-elig", "")
	parentID := s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/parent")

	watcher := s.Env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)
	childID := s.Env.CreateChildWorkspace(t, imported.ProjectID, imported.RepoID, "feature/child", parentID)

	msg := kit.WaitForWorkspace(t, watcher, childID, 5*time.Second, func(m map[string]any) bool {
		can, _ := m["canMergeLocally"].(bool)
		return can
	})
	s.Assert().Equal("feature/parent", msg["parentBranch"])
	s.Assert().Equal(parentID, msg["parentId"])
}

// TestLifecycle_HealthEndpoint verifies the health route responds 200.
func (s *LifecycleSuite) TestLifecycle_HealthEndpoint() {
	t := s.T()

	resp := s.Env.GET(t, "/v0/health")
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestLifecycle_WorkspaceList verifies create then list round-trips over the
// repo-scoped hierarchical list route.
func (s *LifecycleSuite) TestLifecycle_WorkspaceList() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "ws-list", "")

	ids := make([]string, 3)
	for i, branch := range []string{"feature/a", "feature/b", "feature/c"} {
		ids[i] = s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, branch)
	}

	// CreateWorkspace returns once the create is observed on the hub broadcast (WS),
	// but the list route reads the store projection — an independent async read
	// model that can trail the WS frame. Drain asynx so both projections are
	// settled, then assert the snapshot deterministically (no polling, no timeout).
	s.Env.Quiesce()

	listResp := s.Env.GET(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces")
	kit.RequireStatus(t, listResp, http.StatusOK)

	var list []map[string]any
	kit.DecodeEnvData(t, listResp, &list)

	listed := make(map[string]bool, len(list))
	for _, w := range list {
		if id, ok := w["id"].(string); ok {
			listed[id] = true
		}
	}
	for _, wsID := range ids {
		s.Assert().True(listed[wsID], "workspace %q must appear in list", wsID)
	}
}

// TestLifecycle_GitStageCommitChangesStatus proves file edit → stage → commit
// makes the working tree clean again (git status-driven via HTTP) on a writable
// child workspace.
func (s *LifecycleSuite) TestLifecycle_GitStageCommitChangesStatus() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "git-status", "")
	wsID := s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/lifecycle-git")
	base := "/v0/projects/" + imported.ProjectID + "/repos/" + imported.RepoID + "/workspaces/" + wsID

	saveResp := s.Env.PUT(t, base+"/files/content", map[string]any{
		"path":    "hello.txt",
		"content": "Hello, world!\n",
	})
	kit.RequireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	statusResp := s.Env.GET(t, base+"/git/status")
	kit.RequireStatus(t, statusResp, http.StatusOK)

	var statusObj map[string]any
	kit.DecodeEnvData(t, statusResp, &statusObj)

	files, _ := statusObj["files"].([]any)
	s.Assert().NotEmpty(files, "edited file must appear in status")

	stageResp := s.Env.POST(t, base+"/git/stage", map[string]any{
		"paths": []string{"hello.txt"},
	})
	kit.RequireStatus(t, stageResp, http.StatusOK)
	stageResp.Body.Close()

	commitResp := s.Env.POST(t, base+"/git/commit", map[string]any{
		"subject": "Add hello.txt",
		"author":  "Test <t@t.com>",
	})
	kit.RequireStatus(t, commitResp, http.StatusOK)
	commitResp.Body.Close()

	statusResp2 := s.Env.GET(t, base+"/git/status")
	kit.RequireStatus(t, statusResp2, http.StatusOK)

	var statusObj2 map[string]any
	kit.DecodeEnvData(t, statusResp2, &statusObj2)

	files2, _ := statusObj2["files"].([]any)
	s.Assert().Empty(files2, "working tree must be clean after commit")
}
