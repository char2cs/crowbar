//go:build integration

package worktree_test

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// importBody is the import-batch body for branches, each under a fresh chat id.
func importBody(branches ...string) map[string]any {
	rows := make([]map[string]string, 0, len(branches))
	for _, b := range branches {
		rows = append(rows, map[string]string{"name": b, "chatId": uuid.NewString()})
	}
	return map[string]any{"branches": rows}
}

// importBatchURL returns the batch branch-import route, the sole surviving
// import mount (spec §8 step 6 deleted its .../workspaces/import twin).
func (s *WorktreeSuite) importBatchURL() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID + "/chats/import-batch"
}

// TestWorktree_importCreatesWorkspacesForBranches exercises the batch import
// endpoint end-to-end over the real daemon: POST .../chats/import-batch with a
// set of branches returns 202 and each branch's worktree is provisioned and
// linked to its own owning chat. The fixture repo has no GitHub PRs, so this
// pins the no-PR path (each branch forks from the default) — the PR-parenting
// logic is unit-integration-tested against a stubbed PR graph in the worktree
// usecase.
//
// It blocks on each chat's "workspace_set" lifecycle frame rather than a
// worktree_state one carrying the branch: the atomic mint-then-attach sequence
// batch import shares with a single chat create (hierarchyOwningChats.
// AttachOwningWorkspace, container.go — the same pattern
// chat.SpawnChatWithOwnWorktree uses) mints the owning chat BEFORE the
// workspace exists, so the workspace's OWN creation-time push resolves no
// owning chat and is dropped (pushChatWorktree, container.go); attaching the
// chat afterward only emits the terse chat-lifecycle event, never a re-push of
// the linked workspace's git state. So "workspace_set" — not a branch-bearing
// frame — is the only reliable per-branch completion signal this route
// produces; each branch's actual state is then asserted directly on disk.
func (s *WorktreeSuite) TestWorktree_importCreatesWorkspacesForBranches() {
	t := s.T()
	watcher := s.Env.DialRepoChats(t, s.imported.ProjectID, s.imported.RepoID)

	resp := s.Env.POST(t, s.importBatchURL(), importBody("import/alpha", "import/beta"))
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	linked := map[string]bool{}
	watcher.ReadUntil(t, 20*time.Second, func(m map[string]any) bool {
		if m["repoId"] != s.imported.RepoID || m["kind"] != "workspace_set" {
			return false
		}
		if wsID, _ := m["workspaceId"].(string); wsID != "" {
			linked[wsID] = true
		}
		return len(linked) >= 2
	})

	s.Assert().True(kit.BranchExists(t, s.imported.RepoPath, "import/alpha"),
		"import must create branch import/alpha")
	s.Assert().True(kit.BranchExists(t, s.imported.RepoPath, "import/beta"),
		"import must create branch import/beta")
}

// TestWorktree_importHeldBranchYieldsPlaceholder is the black-box regression for
// the confirmed production hang: importing a branch that a worktree OUTSIDE
// Crowbar already has checked out produced nothing at all. git refuses to check
// a branch out twice, CreateChild failed, the batch swallowed it and returned
// success, and the handler's error channel is a no-op without a workspace id —
// so no frame ever reached this stream and the client's optimistic import row,
// which is cleared only by a workspace for that branch, spun forever with
// nothing surfaced.
//
// The contract asserted here is that EVERY imported branch produces a row a
// client can learn of: an unmaterialisable one arrives as a placeholder (no
// localPath, carrying the holder path) that the client can explain, retry and
// detach.
//
// The row is confirmed over REST rather than a branch-bearing WS frame, for
// the same reason TestWorktree_importCreatesWorkspacesForBranches gives:
// batch import mints the owning chat before the workspace exists and only
// broadcasts the terse "workspace_set" link afterward, never the linked
// workspace's own git state (container.go's pushChatWorktree needs the chat
// to already be attached, and nothing re-pushes once it is). "workspace_set"
// firing at all is still the regression's core proof — the old bug was that
// a held branch produced NO frame whatsoever and the client's row spun
// forever; here the chat is reliably linked, and its placeholder shape is
// then read off the row directly.
func (s *WorktreeSuite) TestWorktree_importHeldBranchYieldsPlaceholder() {
	t := s.T()

	// A live worktree outside crowbar home holds the branch — the production
	// trigger was another tool's worktree under ~/.superconductor.
	holderPath := filepath.Join(t.TempDir(), "external-holder")
	kit.GitRun(t, s.imported.RepoPath, "worktree", "add", "-b", "import/held", holderPath)

	watcher := s.Env.DialRepoChats(t, s.imported.ProjectID, s.imported.RepoID)
	resp := s.Env.POST(t, s.importBatchURL(), importBody("import/held"))
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	var chatID string
	watcher.ReadUntil(t, 20*time.Second, func(m map[string]any) bool {
		if m["repoId"] != s.imported.RepoID || m["kind"] != "workspace_set" {
			return false
		}
		if wsID, _ := m["workspaceId"].(string); wsID != "" {
			chatID, _ = m["chatId"].(string)
			return true
		}
		return false
	})
	s.Require().NotEmpty(chatID, "import must link the held branch's workspace to an owning chat")
	s.Env.Quiesce()

	placeholder := s.getWorkspace(chatID)
	s.Require().Equal("import/held", placeholder["branch"],
		"the linked chat must own the import/held branch, not some other row the batch created")

	s.Assert().Empty(placeholder["localPath"],
		"a held branch has no managed worktree — the empty path IS the placeholder signal")
	s.Assert().NotEmpty(placeholder["heldByPath"],
		"the holder path is what the placeholder toast and the Detach… modal render")
	s.Assert().NotEqual("locked", placeholder["status"],
		"an imported feature branch must not be locked; locked survives provisioning "+
			"and would block merge/rename/delete forever")
}

// TestWorktree_importDefaultBranchIsRefusedSynchronously pins the other silent
// hang: the import chain walk terminates AT the default branch, so a request
// naming it created nothing while still answering 202 — leaving the caller's
// optimistic row waiting on a workspace that was never coming. It must be
// refused on the request path, where the caller can surface it.
func (s *WorktreeSuite) TestWorktree_importDefaultBranchIsRefusedSynchronously() {
	t := s.T()

	resp := s.Env.POST(t, s.importBatchURL(), importBody(s.defaultBranch()))
	defer resp.Body.Close()
	kit.RequireStatus(t, resp, http.StatusConflict)
}
