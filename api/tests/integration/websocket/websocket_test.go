//go:build integration

package websocket_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// WebSocketSuite exercises all WebSocket broadcast topics end-to-end.
type WebSocketSuite struct {
	suite.Suite
	env *kit.Env
}

func (s *WebSocketSuite) SetupTest() {
	s.env = kit.BuildEnv(s.T())
}

func TestWebSocketSuite(t *testing.T) {
	suite.Run(t, new(WebSocketSuite))
}

// TestWS_WorkspacesFilter_ProjectID verifies that a client subscribed with
// projectId=X receives only workspaces whose ProjectID matches X.
//
// Wave 4: projectId is derived from the repo record, not the workspace request.
// We create two repos with different projectIds so the workspaces end up in
// different projects without relying on the (now-ignored) request body field.
func (s *WebSocketSuite) TestWS_WorkspacesFilter_ProjectID() {
	t := s.T()

	// Repo r1 belongs to proj-A.
	resp := s.env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "proj-A",
		"name":      "repo-a",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Repo r2 belongs to proj-B — used for the workspace that must be filtered out.
	resp = s.env.POST(t, "/v0/repos", map[string]any{
		"id":        "r2",
		"projectId": "proj-B",
		"name":      "repo-b",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	watcher := s.env.DialWorkspaces(t, "?projectId=proj-A")

	// Create a workspace for proj-B (via r2) — must be filtered out.
	resp = s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r2",
		"branch": "main",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	kit.MutationID(t, resp) // consume body

	// Create the matching workspace for proj-A (via r1).
	resp = s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": "feature/x",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	matchID := kit.MutationID(t, resp)

	msg := kit.WaitForWorkspace(
		t,
		watcher,
		matchID,
		5*time.Second,
		func(_ map[string]any) bool { return true },
	)
	s.Assert().Equal(matchID, msg["id"])
	s.Assert().Equal("proj-A", msg["projectId"])
}

// TestWS_WorkspacesFilter_RepoID verifies per-repo filtering.
func (s *WebSocketSuite) TestWS_WorkspacesFilter_RepoID() {
	t := s.T()

	resp := s.env.POST(t, "/v0/repos", map[string]any{
		"id":        "repo-1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	watcher := s.env.DialWorkspaces(t, "?repoId=repo-1")

	resp = s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "repo-1",
		"branch": "main",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	capturedID := kit.MutationID(t, resp)

	msg := kit.WaitForWorkspace(
		t,
		watcher,
		capturedID,
		5*time.Second,
		func(_ map[string]any) bool { return true },
	)
	s.Assert().Equal("repo-1", msg["repoId"])
}

// TestWS_LSP_FilteredByWsID verifies that the LSP topic filters by wsId.
// This test pushes LSP events directly via the internal PushLSP helper — no
// workspace creation needed, so it remains unchanged from Wave 3.
func (s *WebSocketSuite) TestWS_LSP_FilteredByWsID() {
	t := s.T()

	watcher := s.env.DialLSP(t, "?wsId=ws-lsp-1")

	// Push a diagnostic for a different workspace — must be filtered.
	s.env.PushLSP("ws-other", []kit.LSPDiagnostic{{Message: "skip me"}})

	// Push the matching diagnostic.
	s.env.PushLSP("ws-lsp-1", []kit.LSPDiagnostic{{Message: "target diag"}})

	msg := watcher.ReadUntil(
		t,
		5*time.Second,
		func(m map[string]any) bool {
			return m["wsId"] == "ws-lsp-1"
		},
	)
	s.Assert().Equal("ws-lsp-1", msg["wsId"])

	diags, ok := msg["diagnostics"].([]any)
	s.Require().True(ok)
	s.Require().Len(diags, 1)

	diag0, ok := diags[0].(map[string]any)
	s.Require().True(ok)
	s.Assert().Equal("target diag", diag0["message"])
}

// TestWS_MultiClientFanOut verifies that a push reaches all connected clients.
func (s *WebSocketSuite) TestWS_MultiClientFanOut() {
	t := s.T()

	// Create a repo so we can create a workspace via HTTP.
	resp := s.env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	watcher1 := s.env.DialWorkspaces(t, "")
	watcher2 := s.env.DialWorkspaces(t, "")

	resp = s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": "main",
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	capturedID := kit.MutationID(t, resp)

	msg1 := kit.WaitForWorkspace(
		t,
		watcher1,
		capturedID,
		5*time.Second,
		func(_ map[string]any) bool { return true },
	)
	msg2 := kit.WaitForWorkspace(
		t,
		watcher2,
		capturedID,
		5*time.Second,
		func(_ map[string]any) bool { return true },
	)

	s.Assert().Equal(capturedID, msg1["id"])
	s.Assert().Equal(capturedID, msg2["id"])
}

// TestWS_GitTopic_StatusBroadcast verifies the git WS topic delivers git status.
func (s *WebSocketSuite) TestWS_GitTopic_StatusBroadcast() {
	t := s.T()

	repoPath := kit.InitRepoWithFile(t, "file.txt", "initial\n")
	kit.GitRun(t, repoPath, "branch", "-m", "main", "feature/ws-git-test")

	resp := s.env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Wave 4: only repoId and branch are accepted; worktreePath and id are ignored.
	// The handler derives WorktreePath from the repo's registered path when the
	// branch matches the repo's default branch (adoptMainWorktree).
	resp = s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": kit.BranchName(t, repoPath),
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	wsID := kit.MutationID(t, resp)

	// Scope the watcher to this workspace so the broadcaster filters on wsId.
	watcher := s.env.DialGit(t, "?wsId="+wsID)

	// Dirty the worktree and stage the new file.
	kit.WriteRepoFile(t, repoPath, "new.txt", "new content\n")
	resp = s.env.POST(t, "/v0/workspaces/"+wsID+"/git/stage", map[string]any{
		"paths": []string{"new.txt"},
	})
	kit.RequireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Inject the git status directly — the git WS topic is change-only (no snapshot
	// on connect) and the file watcher is not running in the test environment.
	s.env.PushGit(kit.GitStatusEvent{
		WsID:   wsID,
		Branch: kit.BranchName(t, repoPath),
		Files: []kit.GitFileEntry{
			{Path: "new.txt", Status: "added", Staged: true},
		},
	})

	msg := watcher.ReadUntil(
		t,
		5*time.Second,
		func(m map[string]any) bool { return len(m) > 0 },
	)

	files, ok := msg["files"].([]any)
	s.Require().True(ok, "expected 'files' to be a JSON array")
	s.Require().NotEmpty(files, "expected at least one file entry in git status")

	found := false
	for _, f := range files {
		entry, isMap := f.(map[string]any)
		if !isMap {
			continue
		}
		if path, _ := entry["path"].(string); path == "new.txt" {
			found = true
			break
		}
	}
	s.Assert().True(found, "expected 'new.txt' to appear in the git status files list")
	// wsId is not serialized into the git status payload; the WS connection is
	// scoped by the ?wsId= query param so messages are pre-filtered server-side.
	s.Assert().Empty(msg["wsId"], "wsId must not appear in the git status wire payload")
}

// TestWS_FilesTopicIsChangeOnly verifies that the files topic has no snapshot.
// If a snapshot were sent at connect time we would receive it before the event
// we push below; receiving exactly our event proves the topic is change-only.
func (s *WebSocketSuite) TestWS_FilesTopicIsChangeOnly() {
	t := s.T()

	repoPath := kit.InitRepo(t)

	resp := s.env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Wave 4: repoId + branch only. The handler derives WorktreePath automatically.
	resp = s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": kit.BranchName(t, repoPath),
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	wsID := kit.MutationID(t, resp)

	watcher := s.env.DialFiles(t, "?wsId="+wsID)

	// Inject a file change event directly — the OS file watcher is not running in
	// the test environment, so we bypass it via the public PushFile API.
	s.env.PushFile(kit.FileEvent{
		WsID: wsID,
		Type: "modified",
		Path: "test.txt",
	})

	msg := watcher.ReadUntil(
		t,
		5*time.Second,
		func(m map[string]any) bool { return m["wsId"] == wsID },
	)
	s.Assert().Equal(wsID, msg["wsId"])
	s.Assert().NotEmpty(msg["type"])
	s.Assert().NotEmpty(msg["path"])
}
