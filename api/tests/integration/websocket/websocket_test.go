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

// WebSocketSuite exercises the hierarchical WebSocket prefix-filtering end-to-end.
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

// triggerWorktreeEvent pushes a mock-provider poll result for wsID so the
// workspace aggregate re-broadcasts. It exists because kit.CreateWorkspaceWithChat
// only wires the OWNING CHAT that pushChatWorktree needs to resolve a worktree_state
// frame at all (container.go: "a workspace with no resolved owning chat pushes
// nothing") — the create itself fires before that chat exists (own_worktree.go:
// CreateChildWorkspace runs, THEN SetWorkspace links the chat to it), so its own
// broadcast finds no owner and is silently dropped. The chat feed also carries no
// snapshot (agentChatDef's Snapshot is nil, spec §7.4), so a subscriber dialled
// after creation never replays it either. A provider-state push (the mock seam
// PushProviderState rides) is a real, deterministic workspace mutation that fires
// AFTER the owning chat is in place, which is what actually puts a frame on the
// wire for these prefix-filtering assertions to observe.
func triggerWorktreeEvent(t *testing.T, env *kit.Env, wsID, prURL string) {
	t.Helper()
	env.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    prURL,
		PRTitle:  "t",
	})
}

// TestWS_HierarchicalPrefix_RepoScopeReceivesAllWorkspaces proves that a client
// subscribed at the repo-scoped .../chats/ws prefix (the replacement for the
// deleted repo-scoped `workspaces` stream) receives every worktree under that
// repo via hierarchical prefix matching — no query params (spec §5).
func (s *WebSocketSuite) TestWS_HierarchicalPrefix_RepoScopeReceivesAllWorkspaces() {
	t := s.T()

	imported := s.env.ImportRepo(t, "repo-scope", "")
	wsID, _ := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/x", "")

	watcher := s.env.DialRepoChats(t, imported.ProjectID, imported.RepoID)
	triggerWorktreeEvent(t, s.env, wsID, "https://example.test/pr/1")

	msg := kit.WaitForWorkspace(t, watcher, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	s.Assert().Equal(wsID, msg["id"])
	s.Assert().Equal(imported.RepoID, msg["repoId"])
	// projectId is NOT carried by the chat feed (kit.WorktreeFrame's own doc):
	// the old flat WorkspaceDTO's projectId has no analogue on AgentChatEvent.
	// Scoping to imported.ProjectID is already exercised by DialRepoChats's own
	// URL (project/repo path), so there is nothing more to assert here.
}

// TestWS_HierarchicalPrefix_WsScopeRejectsSibling proves that a client on the
// exact per-chat stream (the replacement for the deleted exact
// .../workspaces/:wsId stream) receives only that chat's worktree frames and
// NOT a sibling's (negative filtering via AssertNoMatch, spec §5).
func (s *WebSocketSuite) TestWS_HierarchicalPrefix_WsScopeRejectsSibling() {
	t := s.T()

	imported := s.env.ImportRepo(t, "ws-scope", "")
	target, targetChatID := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/target", "")

	// Exact-scope subscriber: only frames for `target`'s owning chat should
	// arrive. Confirm the connect snapshot carries the target.
	watcher := s.env.DialChat(t, targetChatID)
	triggerWorktreeEvent(t, s.env, target, "https://example.test/pr/1")
	own := kit.WaitForWorkspace(t, watcher, target, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	s.Assert().Equal(target, own["id"])

	// Create a sibling under the SAME repo and give it a real frame of its own.
	// It must NOT reach the exact-scope subscriber (out-of-prefix). AssertNoMatch
	// reads raw frames, so the predicate must apply kit.WorktreeFrame itself
	// before comparing against "id" (see kit.WaitForWorkspace's own doc on this
	// trap) — a raw chat-feed frame has no top-level "id" at all.
	sibling, _ := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/sibling", "")
	triggerWorktreeEvent(t, s.env, sibling, "https://example.test/pr/2")
	s.Assert().True(
		watcher.AssertNoMatch(t, time.Second, func(m map[string]any) bool {
			return kit.WorktreeFrame(m)["id"] == sibling
		}),
		"exact-scope watcher must not receive a sibling workspace's frame",
	)
}

// TestRegression_Workspaces_NamespaceFiltering proves the full §5 prefix model
// in one test: a repo-scoped subscriber receives the workspace; an exact-scope
// subscriber on a DIFFERENT workspace's chat never does (AssertNoMatch for the
// out-of-prefix id). This is the namespace-filtering contract for the chat feed
// that replaced the Workspaces topic (project/repo/chat prefix).
func (s *WebSocketSuite) TestRegression_Workspaces_NamespaceFiltering() {
	t := s.T()

	imported := s.env.ImportRepo(t, "ns-filter", "")

	// An exact-scope subscriber on the adopted-main workspace's owning chat.
	mainWatcher := s.env.DialChat(t, imported.ChatID)
	triggerWorktreeEvent(t, s.env, imported.WorkspaceID, "https://example.test/pr/main")
	kit.WaitForWorkspace(t, mainWatcher, imported.WorkspaceID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})

	// A repo-scoped subscriber receives every workspace under the repo.
	repoWatcher := s.env.DialRepoChats(t, imported.ProjectID, imported.RepoID)
	wsID, _ := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/ns", "")
	triggerWorktreeEvent(t, s.env, wsID, "https://example.test/pr/ns")
	kit.WaitForWorkspace(t, repoWatcher, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})

	// The exact-scope main subscriber must NEVER receive the sibling's id.
	s.Assert().True(
		mainWatcher.AssertNoMatch(t, time.Second, func(m map[string]any) bool {
			return kit.WorktreeFrame(m)["id"] == wsID
		}),
		"out-of-prefix workspace must not reach an exact-scope subscriber",
	)
}

// TestWS_ProjectScopeReceivesRepoEvents proves that a project-scoped repos
// subscriber receives the repo's RepoDTO via hierarchical prefix matching.
func (s *WebSocketSuite) TestWS_ProjectScopeReceivesRepoEvents() {
	t := s.T()

	repoPath := kit.InitRepo(t)
	projectID := s.env.RegisterProject(t, "proj-scope", repoPath)

	watcher := s.env.DialRepos(t, projectID)
	resp := s.env.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": "repo-scope",
		"path": repoPath,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// The importer derives the repo name from the on-disk directory, so match on
	// the project-scoped prefix (projectId), not the request body name.
	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		return m["projectId"] == projectID && id != ""
	})
	s.Assert().Equal(projectID, msg["projectId"])
}

// TestWS_MultiClientFanOut verifies that a worktree state change reaches all
// connected clients on the same repo-scoped prefix.
func (s *WebSocketSuite) TestWS_MultiClientFanOut() {
	t := s.T()

	imported := s.env.ImportRepo(t, "fan-out", "")
	wsID, _ := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/fan", "")

	watcher1 := s.env.DialRepoChats(t, imported.ProjectID, imported.RepoID)
	watcher2 := s.env.DialRepoChats(t, imported.ProjectID, imported.RepoID)

	triggerWorktreeEvent(t, s.env, wsID, "https://example.test/pr/fan")

	msg1 := kit.WaitForWorkspace(t, watcher1, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	msg2 := kit.WaitForWorkspace(t, watcher2, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	s.Assert().Equal(wsID, msg1["id"])
	s.Assert().Equal(wsID, msg2["id"])
}
