//go:build integration

package git_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

type GitSuite struct {
	suite.Suite
	Env      *kit.Env
	imported kit.ImportedRepo
	wsID     string
	chatID   string
	worktree string
}

func (s *GitSuite) SetupTest() {
	s.Env = kit.BuildEnv(s.T())
	s.imported = s.Env.ImportRepo(s.T(), "git", "")
	s.wsID, s.chatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/git-test", "",
	)
	s.worktree = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)
}

func (s *GitSuite) base() string {
	return "/v0/chats/" + s.chatID
}

func TestGitSuite(t *testing.T) {
	suite.Run(
		t,
		new(GitSuite),
	)
}

func (s *GitSuite) gitStatus() map[string]any {
	resp := s.Env.GET(s.T(), s.base()+"/git/status")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var r map[string]any
	kit.DecodeEnvData(s.T(), resp, &r)
	return r
}

func (s *GitSuite) stageFile(path string) {
	resp := s.Env.POST(s.T(), s.base()+"/git/stage", map[string]any{"paths": []string{path}})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()
}

func (s *GitSuite) commit(msg string) {
	resp := s.Env.POST(s.T(), s.base()+"/git/commit", map[string]any{"subject": msg, "author": "Test <t@t.com>"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()
}

func statusFilePaths(status map[string]any) []string {
	raw, _ := status["files"].([]any)
	paths := make([]string, 0, len(raw))
	for _, f := range raw {
		if m, ok := f.(map[string]any); ok {
			if p, ok := m["path"].(string); ok {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

func statusFiles(status map[string]any) []map[string]any {
	raw, _ := status["files"].([]any)
	result := make([]map[string]any, 0, len(raw))
	for _, f := range raw {
		if m, ok := f.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

func (s *GitSuite) TestGit_statusReflectsUntrackedFile() {
	t := s.T()

	kit.WriteRepoFile(t, s.worktree, "untracked.txt", "hello\n")

	status := s.gitStatus()
	s.Require().NotEmpty(statusFiles(status))
	s.Assert().Contains(statusFilePaths(status), "untracked.txt")
}

func (s *GitSuite) TestGit_stageFile() {
	t := s.T()

	kit.WriteRepoFile(t, s.worktree, "staged.txt", "content\n")
	s.stageFile("staged.txt")

	files := statusFiles(s.gitStatus())
	s.Require().Len(files, 1)
	s.Assert().True(files[0]["staged"].(bool), "file must be staged after StageFile")
}

func (s *GitSuite) TestGit_unstageFile() {
	t := s.T()

	kit.WriteRepoFile(t, s.worktree, "unstage.txt", "content\n")
	s.stageFile("unstage.txt")

	resp := s.Env.POST(s.T(), s.base()+"/git/unstage", map[string]any{"paths": []string{"unstage.txt"}})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	files := statusFiles(s.gitStatus())
	s.Require().Len(files, 1)
	s.Assert().False(files[0]["staged"].(bool), "file must be unstaged after UnstageFile")
}

func (s *GitSuite) TestGit_commitClearsStatus() {
	t := s.T()

	kit.WriteRepoFile(t, s.worktree, "commit.txt", "commit me\n")
	s.stageFile("commit.txt")
	s.commit("Add commit.txt")

	s.Assert().Empty(statusFiles(s.gitStatus()), "working tree must be clean after commit")
}

func (s *GitSuite) TestGit_diffShowsUnstagedChanges() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "base.txt", "line1\nline2\n", "base commit")
	kit.WriteRepoFile(t, s.worktree, "base.txt", "line1\nchanged\n")

	resp := s.Env.GET(s.T(), s.base()+"/git/diff?staged=false")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var diffs []map[string]any
	kit.DecodeEnvData(s.T(), resp, &diffs)

	s.Require().NotEmpty(diffs)
	s.Assert().Equal("base.txt", diffs[0]["file_path"].(string))
	hunks, _ := diffs[0]["hunks"].([]any)
	s.Assert().NotEmpty(hunks)
}

func (s *GitSuite) TestGit_logShowsCommit() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "log.txt", "content\n", "log test commit")

	resp := s.Env.GET(s.T(), s.base()+"/git/log?limit=10&skip=0")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var commits []map[string]any
	kit.DecodeEnvData(s.T(), resp, &commits)

	s.Require().NotEmpty(commits)
	s.Assert().Equal("log test commit", commits[0]["message"].(string))
}

func (s *GitSuite) TestGit_stashPushAndPop() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "stash.txt", "original\n", "base")
	kit.WriteRepoFile(t, s.worktree, "stash.txt", "modified\n")

	resp := s.Env.POST(s.T(), s.base()+"/git/stash", map[string]any{"message": "wip changes"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/git/stashes")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var stashes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &stashes)
	s.Require().Len(stashes, 1)

	resp = s.Env.POST(s.T(), s.base()+"/git/stash-pop", map[string]any{"index": 0})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/git/stashes")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var stashesAfter []map[string]any
	kit.DecodeEnvData(s.T(), resp, &stashesAfter)
	s.Assert().Empty(stashesAfter, "stash list must be empty after pop")
}

func (s *GitSuite) TestGit_stashDrop() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "drop.txt", "original\n", "base")
	kit.WriteRepoFile(t, s.worktree, "drop.txt", "modified\n")

	resp := s.Env.POST(s.T(), s.base()+"/git/stash", map[string]any{"message": "to drop"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/git/stashes")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var stashes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &stashes)
	s.Require().Len(stashes, 1)

	resp = s.Env.DELETE(s.T(), s.base()+"/git/stash?index=0")
	kit.RequireStatus(s.T(), resp, http.StatusNoContent)

	resp = s.Env.GET(s.T(), s.base()+"/git/stashes")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var stashesAfter []map[string]any
	kit.DecodeEnvData(s.T(), resp, &stashesAfter)
	s.Assert().Empty(stashesAfter)
}

func (s *GitSuite) TestGit_branchesListsCurrent() {
	t := s.T()

	currentBranch := kit.BranchName(t, s.worktree)

	resp := s.Env.GET(s.T(), s.base()+"/git/branches")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var branches []map[string]any
	kit.DecodeEnvData(s.T(), resp, &branches)
	s.Require().NotEmpty(branches)

	s.Assert().True(branchIsCurrent(branches, currentBranch),
		"current branch %q must appear in Branches with IsCurrent=true", currentBranch)
}

func (s *GitSuite) TestGit_blameAnnotatesLines() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "blame.txt", "line1\nline2\nline3\n", "blame base")

	resp := s.Env.GET(s.T(), s.base()+"/blame?path=blame.txt") //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var entries []map[string]any
	kit.DecodeEnvData(s.T(), resp, &entries)

	s.Require().GreaterOrEqual(len(entries), 3)
	s.Assert().Equal(float64(1), entries[0]["lineNumber"])
}

func (s *GitSuite) TestGit_hunkStageAndUnstage() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "hunky.txt", "line1\nline2\nline3\n", "base")
	kit.WriteRepoFile(t, s.worktree, "hunky.txt", "CHANGED\nline2\nline3\n")

	resp := s.Env.GET(s.T(), s.base()+"/git/diff?staged=false")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var diffs []map[string]any
	kit.DecodeEnvData(s.T(), resp, &diffs)
	s.Require().NotEmpty(diffs)
	hunks, _ := diffs[0]["hunks"].([]any)
	s.Require().NotEmpty(hunks)
	firstHunk, _ := hunks[0].(map[string]any)
	hunkID := firstHunk["hunkId"].(string)

	resp = s.Env.POST(s.T(), s.base()+"/git/stage-hunk", map[string]any{
		"path":   "hunky.txt",
		"hunkId": hunkID,
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/git/diff?staged=true")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var stagedDiffs []map[string]any
	kit.DecodeEnvData(s.T(), resp, &stagedDiffs)
	s.Require().NotEmpty(stagedDiffs, "staged diff must be non-empty after hunk stage")

	resp = s.Env.POST(s.T(), s.base()+"/git/unstage-hunk", map[string]any{
		"path":   "hunky.txt",
		"hunkId": hunkID,
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/git/diff?staged=true")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var stagedAfter []map[string]any
	kit.DecodeEnvData(s.T(), resp, &stagedAfter)
	s.Assert().Empty(stagedAfter, "staged diff must be empty after hunk unstage")
}

func branchIsCurrent(
	branches []map[string]any,
	name string,
) bool {
	for _, b := range branches {
		if b["name"] == name {
			if ic, ok := b["isCurrent"].(bool); ok && ic {
				return true
			}
		}
	}
	return false
}

func (s *GitSuite) TestGit_resetRestoresCleanTree() {
	t := s.T()

	kit.CommitFile(t, s.worktree, "reset.txt", "original\n", "before reset")
	kit.WriteRepoFile(t, s.worktree, "reset.txt", "dirty\n")

	resp := s.Env.POST(s.T(), s.base()+"/git/reset", map[string]any{"mode": "hard"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	s.Assert().Empty(statusFiles(s.gitStatus()), "working tree must be clean after hard reset")
}
