//go:build integration

package worktree_test

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
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

// linkedWorkspace answers the workspace a chat row of this repo is linked to,
// other than the adopted main one, or "".
func (s *WorktreeSuite) linkedWorkspace(chat map[string]any) string {
	wsID, _ := chat["workspaceId"].(string)
	if chat["repoId"] != s.imported.RepoID || wsID == s.imported.WorkspaceID {
		return ""
	}
	return wsID
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
// It blocks on each new chat's entity naming its linked workspace: batch
// import mints the owning chat before the workspace and attaches it after, so
// the link is the per-branch completion signal; each branch is then checked on disk.
func (s *WorktreeSuite) TestWorktree_importCreatesWorkspacesForBranches() {
	t := s.T()
	watcher := s.Env.HoldProjects(t)

	resp := s.Env.POST(t, s.importBatchURL(), importBody("import/alpha", "import/beta"))
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	linked := map[string]bool{}
	watcher.Next(t, 20*time.Second, stream.KindChatEntity, func(m map[string]any) bool {
		if wsID := s.linkedWorkspace(m); wsID != "" {
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
// The link reaching the stream at all is the regression's core proof (the old
// bug produced no row whatsoever); the placeholder's shape is then read off
// the REST row.
func (s *WorktreeSuite) TestWorktree_importHeldBranchYieldsPlaceholder() {
	t := s.T()

	// A live worktree outside crowbar home holds the branch — the production
	// trigger was another tool's worktree under ~/.superconductor.
	holderPath := filepath.Join(t.TempDir(), "external-holder")
	kit.GitRun(t, s.imported.RepoPath, "worktree", "add", "-b", "import/held", holderPath)

	watcher := s.Env.HoldProjects(t)
	resp := s.Env.POST(t, s.importBatchURL(), importBody("import/held"))
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	var chatID string
	watcher.Next(t, 20*time.Second, stream.KindChatEntity, func(m map[string]any) bool {
		if s.linkedWorkspace(m) == "" {
			return false
		}
		chatID, _ = m["id"].(string)
		return true
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
