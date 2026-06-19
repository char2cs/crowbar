//go:build integration

package lifecycle_test

import (
	"fmt"
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

// TestLifecycle_WorkspaceCreateBroadcastsOverWS proves the full path:
// POST /v0/workspaces → projection → hub.BroadcastWorkspace → WS client.
func (s *LifecycleSuite) TestLifecycle_WorkspaceCreateBroadcastsOverWS() {
	t := s.T()

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	watcher := s.Env.DialWorkspaces(t, "?projectId=p1")

	resp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": "feature/lifecycle",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	wsID := kit.MutationID(t, resp)

	msg := kit.WaitForWorkspace(
		t,
		watcher,
		wsID,
		5*time.Second,
		func(_ map[string]any) bool { return true },
	)
	s.Assert().Equal(wsID, msg["id"])
	s.Assert().Equal("feature/lifecycle", msg["branch"])
	s.Assert().Equal("new", msg["status"])
}

// TestLifecycle_WorkingTreeSyncUpdatesReadModelAndBroadcasts proves that
// POST /v0/workspaces/:id/sync pushes an updated row over WS with cleared status badge.
func (s *LifecycleSuite) TestLifecycle_WorkingTreeSyncUpdatesReadModelAndBroadcasts() {
	t := s.T()

	repoPath := kit.InitRepo(t)
	kit.GitRun(t, repoPath, "branch", "-m", "main", "feature/lifecycle-sync")

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	watcher := s.Env.DialWorkspaces(t, "?projectId=p1")

	resp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": kit.BranchName(t, repoPath),
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	wsID := kit.MutationID(t, resp)

	_ = kit.WaitForWorkspace(
		t,
		watcher,
		wsID,
		5*time.Second,
		func(m map[string]any) bool {
			return m["status"] == "new"
		},
	)

	kit.WriteRepoFile(t, repoPath, "a.txt", "aaa")
	kit.WriteRepoFile(t, repoPath, "b.txt", "bbb")
	kit.WriteRepoFile(t, repoPath, "c.txt", "ccc")

	// Stage and commit so HasCommits=true — sync only clears the "new" badge when
	// the workspace has at least one commit ahead of its fork point.
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		stageResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/git/stage", map[string]any{
			"paths": []string{f},
		})
		kit.RequireStatus(t, stageResp, http.StatusOK)
		stageResp.Body.Close()
	}
	commitResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/git/commit", map[string]any{
		"subject": "add sync test files",
		"author":  "Test <t@t.com>",
	})
	kit.RequireStatus(t, commitResp, http.StatusOK)
	commitResp.Body.Close()

	syncResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/sync", nil)
	kit.RequireStatus(t, syncResp, http.StatusOK)
	syncResp.Body.Close()

	msg := kit.WaitForWorkspace(
		t,
		watcher,
		wsID,
		5*time.Second,
		func(m map[string]any) bool {
			_, hasStatus := m["status"]
			return !hasStatus
		},
	)
	_, hasStatus := msg["status"]
	s.Assert().False(hasStatus, "status badge must be absent (omitempty) once HasCommits is true and synced")
}

// TestLifecycle_HealthEndpoint verifies the health route responds 200.
func (s *LifecycleSuite) TestLifecycle_HealthEndpoint() {
	t := s.T()

	resp := s.Env.GET(t, "/v0/health")
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestLifecycle_WorkspaceList verifies create then list round-trips.
func (s *LifecycleSuite) TestLifecycle_WorkspaceList() {
	t := s.T()

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	ids := make([]string, 3)
	for i, branch := range []string{"feature/a", "feature/b", "feature/c"} {
		resp := s.Env.POST(t, "/v0/workspaces", map[string]any{
			"repoId": "r1",
			"branch": fmt.Sprintf("%s-%d", branch, i),
		})
		kit.RequireStatus(t, resp, http.StatusCreated)
		ids[i] = kit.MutationID(t, resp)
	}

	listResp := s.Env.GET(t, "/v0/workspaces")
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
// makes the working tree clean again (git status-driven via HTTP).
func (s *LifecycleSuite) TestLifecycle_GitStageCommitChangesStatus() {
	t := s.T()

	repoPath := kit.InitRepo(t)
	kit.GitRun(t, repoPath, "branch", "-m", "main", "feature/lifecycle-git")

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	wsResp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": kit.BranchName(t, repoPath),
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	wsID := kit.MutationID(t, wsResp)

	kit.WriteRepoFile(t, repoPath, "hello.txt", "Hello, world!\n")

	statusResp := s.Env.GET(t, "/v0/workspaces/"+wsID+"/git/status")
	kit.RequireStatus(t, statusResp, http.StatusOK)

	var statusObj map[string]any
	kit.DecodeEnvData(t, statusResp, &statusObj)

	files, _ := statusObj["files"].([]any)
	s.Assert().NotEmpty(files, "untracked file must appear in status")

	paths := make([]string, 0, len(files))
	for _, f := range files {
		fm, _ := f.(map[string]any)
		if p, ok := fm["path"].(string); ok {
			paths = append(paths, p)
		}
	}
	s.Assert().Contains(paths, "hello.txt")

	stageResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/git/stage", map[string]any{
		"paths": []string{"hello.txt"},
	})
	kit.RequireStatus(t, stageResp, http.StatusOK)
	stageResp.Body.Close()

	commitResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/git/commit", map[string]any{
		"subject": "Add hello.txt",
		"author":  "Test <t@t.com>",
	})
	kit.RequireStatus(t, commitResp, http.StatusOK)
	commitResp.Body.Close()

	statusResp2 := s.Env.GET(t, "/v0/workspaces/"+wsID+"/git/status")
	kit.RequireStatus(t, statusResp2, http.StatusOK)

	var statusObj2 map[string]any
	kit.DecodeEnvData(t, statusResp2, &statusObj2)

	files2, _ := statusObj2["files"].([]any)
	s.Assert().Empty(files2, "working tree must be clean after commit")
}
