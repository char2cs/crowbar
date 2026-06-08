//go:build integration

package git_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test harness entry point.
func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

// GitSuite tests the git read and write usecase surface against a real git repo.
type GitSuite struct {
	suite.Suite
	Env      *kit.Env
	repoPath string
	wsID     string
}

// SetupTest initialises a fresh environment, repo, and workspace for each git test case.
func (s *GitSuite) SetupTest() {
	s.Env = kit.BuildEnv(s.T())
	s.repoPath = kit.InitRepo(s.T())
	kit.GitRun(s.T(), s.repoPath, "branch", "-m", "main", "feature/git-test")

	resp := s.Env.POST(s.T(), "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "test-repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), resp, 201)
	resp.Body.Close()

	resp = s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": kit.BranchName(s.T(), s.repoPath),
	})
	kit.RequireStatus(s.T(), resp, 201)
	s.wsID = kit.MutationID(s.T(), resp)
}

// TestGitSuite runs the git integration suite.
func TestGitSuite(t *testing.T) {
	suite.Run(
		t,
		new(GitSuite),
	)
}

// gitStatus calls GET /v0/workspaces/:id/git/status and returns the decoded response.
func (s *GitSuite) gitStatus() map[string]any {
	resp := s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/status")
	kit.RequireStatus(s.T(), resp, 200)
	var r map[string]any
	kit.DecodeJSON(s.T(), resp, &r)
	return r
}

// stageFile calls POST /v0/workspaces/:id/git/stage for the given path.
func (s *GitSuite) stageFile(path string) {
	resp := s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/stage", map[string]any{"path": path})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()
}

// commit calls POST /v0/workspaces/:id/git/commit with the given message.
func (s *GitSuite) commit(msg string) {
	resp := s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/commit", map[string]any{"message": msg, "author": "Test <t@t.com>"})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()
}

// statusFilePaths extracts the list of file paths from a gitStatus response.
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

// statusFiles returns the files slice from a gitStatus response.
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

// TestGit_statusReflectsUntrackedFile verifies git status detects new files.
func (s *GitSuite) TestGit_statusReflectsUntrackedFile() {
	t := s.T()

	kit.WriteRepoFile(
		t,
		s.repoPath,
		"untracked.txt",
		"hello\n",
	)

	status := s.gitStatus()
	files := statusFiles(status)
	s.Require().NotEmpty(files)
	paths := statusFilePaths(status)
	s.Assert().Contains(
		paths,
		"untracked.txt",
	)
}

// TestGit_stageFile moves a file from untracked to staged.
func (s *GitSuite) TestGit_stageFile() {
	t := s.T()

	kit.WriteRepoFile(
		t,
		s.repoPath,
		"staged.txt",
		"content\n",
	)
	s.stageFile("staged.txt")

	status := s.gitStatus()
	files := statusFiles(status)
	s.Require().Len(files, 1)
	s.Assert().True(
		files[0]["staged"].(bool),
		"file must be staged after StageFile",
	)
}

// TestGit_unstageFile moves a staged file back to unstaged.
func (s *GitSuite) TestGit_unstageFile() {
	t := s.T()

	kit.WriteRepoFile(
		t,
		s.repoPath,
		"unstage.txt",
		"content\n",
	)
	s.stageFile("unstage.txt")

	resp := s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/unstage", map[string]any{"path": "unstage.txt"})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	status := s.gitStatus()
	files := statusFiles(status)
	s.Require().Len(files, 1)
	s.Assert().False(
		files[0]["staged"].(bool),
		"file must be unstaged after UnstageFile",
	)
}

// TestGit_commitClearsStatus verifies stage + commit produces a clean tree.
func (s *GitSuite) TestGit_commitClearsStatus() {
	t := s.T()

	kit.WriteRepoFile(
		t,
		s.repoPath,
		"commit.txt",
		"commit me\n",
	)
	s.stageFile("commit.txt")
	s.commit("Add commit.txt")

	status := s.gitStatus()
	files := statusFiles(status)
	s.Assert().Empty(
		files,
		"working tree must be clean after commit",
	)
}

// TestGit_diffShowsUnstagedChanges verifies diff returns hunk data for modified files.
func (s *GitSuite) TestGit_diffShowsUnstagedChanges() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"base.txt",
		"line1\nline2\n",
		"base commit",
	)

	kit.WriteRepoFile(
		t,
		s.repoPath,
		"base.txt",
		"line1\nchanged\n",
	)

	resp := s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/diff?staged=false")
	kit.RequireStatus(s.T(), resp, 200)
	var diffs []map[string]any
	kit.DecodeJSON(s.T(), resp, &diffs)

	s.Require().NotEmpty(diffs)
	s.Assert().Equal(
		"base.txt",
		diffs[0]["file_path"].(string),
	)
	hunks, _ := diffs[0]["hunks"].([]any)
	s.Assert().NotEmpty(hunks)
}

// TestGit_logShowsCommit verifies log returns the committed history.
func (s *GitSuite) TestGit_logShowsCommit() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"log.txt",
		"content\n",
		"log test commit",
	)

	resp := s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/log?limit=10&skip=0")
	kit.RequireStatus(s.T(), resp, 200)
	var commits []map[string]any
	kit.DecodeJSON(s.T(), resp, &commits)

	s.Require().NotEmpty(commits)
	s.Assert().Equal(
		"log test commit",
		commits[0]["message"].(string),
	)
}

// TestGit_stashPushAndPop verifies stash round-trips.
func (s *GitSuite) TestGit_stashPushAndPop() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"stash.txt",
		"original\n",
		"base",
	)
	kit.WriteRepoFile(
		t,
		s.repoPath,
		"stash.txt",
		"modified\n",
	)

	resp := s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/stash", map[string]any{"message": "wip changes"})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/stashes")
	kit.RequireStatus(s.T(), resp, 200)
	var stashes []map[string]any
	kit.DecodeJSON(s.T(), resp, &stashes)
	s.Require().Len(stashes, 1)

	resp = s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/stash-pop", map[string]any{"index": 0})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/stashes")
	kit.RequireStatus(s.T(), resp, 200)
	var stashesAfter []map[string]any
	kit.DecodeJSON(s.T(), resp, &stashesAfter)
	s.Assert().Empty(
		stashesAfter,
		"stash list must be empty after pop",
	)
}

// TestGit_stashDrop removes a stash entry without applying it.
func (s *GitSuite) TestGit_stashDrop() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"drop.txt",
		"original\n",
		"base",
	)
	kit.WriteRepoFile(
		t,
		s.repoPath,
		"drop.txt",
		"modified\n",
	)

	resp := s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/stash", map[string]any{"message": "to drop"})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/stashes")
	kit.RequireStatus(s.T(), resp, 200)
	var stashes []map[string]any
	kit.DecodeJSON(s.T(), resp, &stashes)
	s.Require().Len(stashes, 1)

	resp = s.Env.DELETE(s.T(), "/v0/workspaces/"+s.wsID+"/git/stash?index=0")
	kit.RequireStatus(s.T(), resp, 204)

	resp = s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/stashes")
	kit.RequireStatus(s.T(), resp, 200)
	var stashesAfter []map[string]any
	kit.DecodeJSON(s.T(), resp, &stashesAfter)
	s.Assert().Empty(stashesAfter)
}

// TestGit_branchesListsCurrent verifies Branches returns the current branch.
func (s *GitSuite) TestGit_branchesListsCurrent() {
	t := s.T()

	currentBranch := kit.BranchName(
		t,
		s.repoPath,
	)

	resp := s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/branches")
	kit.RequireStatus(s.T(), resp, 200)
	var branches []map[string]any
	kit.DecodeJSON(s.T(), resp, &branches)
	s.Require().NotEmpty(branches)

	isCurrent := branchIsCurrent(
		branches,
		currentBranch,
	)
	s.Assert().True(
		isCurrent,
		"current branch %q must appear in Branches with IsCurrent=true",
		currentBranch,
	)
}

// TestGit_blameAnnotatesLines verifies blame returns one entry per line.
func (s *GitSuite) TestGit_blameAnnotatesLines() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"blame.txt",
		"line1\nline2\nline3\n",
		"blame base",
	)

	resp := s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/blame?path=blame.txt")
	kit.RequireStatus(s.T(), resp, 200)
	var entries []map[string]any
	kit.DecodeJSON(s.T(), resp, &entries)

	s.Require().GreaterOrEqual(len(entries), 3)
	lineNum := entries[0]["lineNumber"]
	// lineNumber is decoded as float64 from JSON
	s.Assert().Equal(
		float64(1),
		lineNum,
	)
}

// TestGit_hunkStageAndUnstage verifies hunk-level staging round-trips.
func (s *GitSuite) TestGit_hunkStageAndUnstage() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"hunky.txt",
		"line1\nline2\nline3\n",
		"base",
	)
	kit.WriteRepoFile(
		t,
		s.repoPath,
		"hunky.txt",
		"CHANGED\nline2\nline3\n",
	)

	resp := s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/diff?staged=false")
	kit.RequireStatus(s.T(), resp, 200)
	var diffs []map[string]any
	kit.DecodeJSON(s.T(), resp, &diffs)
	s.Require().NotEmpty(diffs)
	hunks, _ := diffs[0]["hunks"].([]any)
	s.Require().NotEmpty(hunks)
	firstHunk, _ := hunks[0].(map[string]any)
	hunkID := firstHunk["hunkId"].(string)

	resp = s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/stage-hunk", map[string]any{
		"path":   "hunky.txt",
		"hunkId": hunkID,
	})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/diff?staged=true")
	kit.RequireStatus(s.T(), resp, 200)
	var stagedDiffs []map[string]any
	kit.DecodeJSON(s.T(), resp, &stagedDiffs)
	s.Require().NotEmpty(
		stagedDiffs,
		"staged diff must be non-empty after hunk stage",
	)

	resp = s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/unstage-hunk", map[string]any{
		"path":   "hunky.txt",
		"hunkId": hunkID,
	})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), "/v0/workspaces/"+s.wsID+"/git/diff?staged=true")
	kit.RequireStatus(s.T(), resp, 200)
	var stagedAfter []map[string]any
	kit.DecodeJSON(s.T(), resp, &stagedAfter)
	s.Assert().Empty(
		stagedAfter,
		"staged diff must be empty after hunk unstage",
	)
}

// branchIsCurrent reports whether name appears in branches with IsCurrent==true.
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

// TestGit_resetRestoresCleanTree verifies hard reset clears uncommitted changes.
func (s *GitSuite) TestGit_resetRestoresCleanTree() {
	t := s.T()

	kit.CommitFile(
		t,
		s.repoPath,
		"reset.txt",
		"original\n",
		"before reset",
	)
	kit.WriteRepoFile(
		t,
		s.repoPath,
		"reset.txt",
		"dirty\n",
	)

	resp := s.Env.POST(s.T(), "/v0/workspaces/"+s.wsID+"/git/reset", map[string]any{"mode": "hard"})
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	status := s.gitStatus()
	files := statusFiles(status)
	s.Assert().Empty(
		files,
		"working tree must be clean after hard reset",
	)
}

