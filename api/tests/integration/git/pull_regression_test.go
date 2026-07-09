//go:build integration

package git_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// ensureOrigin points repo's `origin` remote at url, tolerating a pre-existing
// origin (the import may already have wired one).
func ensureOrigin(
	t *testing.T,
	repo string,
	url string,
) {
	t.Helper()
	rm := exec.Command("git", "remote", "remove", "origin")
	rm.Dir = repo
	rm.Env = append(os.Environ(), kit.GitEnv...)
	_ = rm.Run()
	kit.GitRun(t, repo, "remote", "add", "origin", url)
}

// mergeInProgress reports whether repo has an in-progress merge (a MERGE_HEAD),
// resolving the marker correctly even for a linked worktree whose .git is a
// gitlink file. It is the on-disk proof that a refused pull never wedged the tree.
func mergeInProgress(
	t *testing.T,
	repo string,
) bool {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "-q", "--verify", "MERGE_HEAD")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), kit.GitEnv...)
	return cmd.Run() == nil
}

// advanceRemoteBranch clones bare, advances branch with a fresh commit, pushes it
// back, and returns the new tip SHA. This moves origin/<branch> ahead without
// touching the workspace worktree, so the worktree can then diverge (a local
// commit) or trail (no local commit) behind it.
func advanceRemoteBranch(
	t *testing.T,
	bare string,
	branch string,
	content string,
	msg string,
) string {
	t.Helper()
	parent := t.TempDir()
	clone := filepath.Join(parent, "clone")
	kit.GitRun(t, parent, "clone", bare, clone)
	kit.GitRun(t, clone, "checkout", branch)
	kit.CommitFile(t, clone, "shared.txt", content, msg)
	kit.GitRun(t, clone, "push", "origin", branch)
	return kit.RevParse(t, clone, "HEAD")
}

// wireBareOrigin creates a bare origin for the workspace worktree, wires it as
// `origin`, and pushes the current branch to it with upstream tracking (so a
// later `git pull` knows what to pull). It returns the bare repo path.
func (s *GitSuite) wireBareOrigin(
	branch string,
) string {
	t := s.T()
	bare := t.TempDir()
	kit.GitRun(t, bare, "init", "--bare")
	ensureOrigin(t, s.worktree, bare)
	kit.GitRun(t, s.worktree, "push", "-u", "origin", branch)
	return bare
}

// decodeErrorEnvelope reads the error string from a Crowbar error envelope.
func decodeErrorEnvelope(
	t *testing.T,
	resp *http.Response,
) string {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	return env.Error
}

// TestRegression_PullRefusesNonFastForward pins the safe-only pull contract: a
// branch that has diverged from its origin upstream (a local commit the upstream
// lacks) is refused synchronously with 409 `not_fast_forwardable`, and the
// worktree is left untouched — no blind merge, no MERGE_HEAD, no unmerged paths.
// This is the regression for the wedged-tree bug: the old `git pull --no-rebase`
// merged the divergent histories and left the worktree conflicted with no UI.
func (s *GitSuite) TestRegression_PullRefusesNonFastForward() {
	t := s.T()

	branch := kit.BranchName(t, s.worktree)
	bare := s.wireBareOrigin(branch)

	// Origin advances, and the worktree makes its own commit on the same file:
	// the two histories diverge, so a pull cannot fast-forward.
	advanceRemoteBranch(t, bare, branch, "REMOTE\n", "remote change")
	kit.CommitFile(t, s.worktree, "shared.txt", "LOCAL\n", "local change")
	kit.GitRun(t, s.worktree, "fetch", "origin")

	localTip := kit.RevParse(t, s.worktree, "HEAD")

	resp := s.Env.POST(t, s.base()+"/git/pull", map[string]any{})
	kit.RequireStatus(t, resp, http.StatusConflict)
	s.Assert().Equal("not_fast_forwardable", decodeErrorEnvelope(t, resp))

	// The refusal must leave the worktree exactly as it was: still on its own tip,
	// clean working tree, and no merge in progress.
	s.Assert().Equal(localTip, kit.RevParse(t, s.worktree, "HEAD"), "a refused pull must not move HEAD")
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(t, s.worktree, "status", "--porcelain")),
		"a refused pull must leave a clean working tree (no unmerged paths)")
	s.Assert().False(mergeInProgress(t, s.worktree), "a refused pull must not leave a MERGE_HEAD")
}

// TestRegression_PullAllowsFastForward is the sibling contract: a branch that is
// strictly behind its upstream (no local commits the upstream lacks) can fast
// forward, so the pull is accepted (202) and runs asynchronously, advancing the
// worktree to the upstream tip.
func (s *GitSuite) TestRegression_PullAllowsFastForward() {
	t := s.T()

	branch := kit.BranchName(t, s.worktree)
	bare := s.wireBareOrigin(branch)

	// Origin advances; the worktree makes no local commit, so it only trails.
	remoteTip := advanceRemoteBranch(t, bare, branch, "REMOTE\n", "remote change")
	kit.GitRun(t, s.worktree, "fetch", "origin")

	watcher := s.Env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	resp := s.Env.POST(t, s.base()+"/git/pull", map[string]any{})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Drain the async pull deterministically via the working overlay (true→false),
	// then confirm it fast-forwarded the worktree to the upstream tip.
	kit.WaitForWorkspace(t, watcher, s.wsID, 10*time.Second, func(m map[string]any) bool {
		return m["working"] == true
	})
	kit.WaitForWorkspace(t, watcher, s.wsID, 10*time.Second, func(m map[string]any) bool {
		return m["working"] == false
	})
	s.Assert().Equal(remoteTip, kit.RevParse(t, s.worktree, "HEAD"), "an accepted ff pull must advance HEAD to the upstream tip")
}
