//go:build integration

package git

import (
	"os"
	"os/exec"
	"testing"

	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestGit(t *testing.T) { suite.Run(t, new(GitSuite)) }

type GitSuite struct{ kit.IntegrationSuite }

// TestGitLogReturnsAllCommits creates 3 commits on a GitRepo, creates a Task
// on that repository, and asserts GetGitLog returns all commits in order.
func (s *GitSuite) TestGitLogReturnsAllCommits() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	sha1 := gitRepo.Commit("first commit", map[string]string{"a.txt": "aaa"})
	sha2 := gitRepo.Commit("second commit", map[string]string{"b.txt": "bbb"})
	sha3 := gitRepo.Commit("third commit", map[string]string{"c.txt": "ccc"})

	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "git-log-branch")

	log := env.Client.GetGitLog(task.ID)
	require.GreaterOrEqual(s.T(), len(log), 3)

	shas := make(map[string]bool, len(log))
	for _, c := range log {
		shas[c.SHA] = true
	}
	assert.True(s.T(), shas[sha1], "sha1 not in log")
	assert.True(s.T(), shas[sha2], "sha2 not in log")
	assert.True(s.T(), shas[sha3], "sha3 not in log")
}

// TestGitDiffReturnsChangedFiles makes a commit inside the task's worktree and
// asserts GetGitDiff reports the correct file name and hunk count.
func (s *GitSuite) TestGitDiffReturnsChangedFiles() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	gitRepo.Commit("baseline", map[string]string{"README.md": "# Readme\n"})

	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "git-diff-branch")

	// Write a file into the worktree and commit it.
	filePath := task.WorktreePath + "/new_feature.go"
	require.NoError(s.T(), os.WriteFile(filePath, []byte("package main\n\nfunc feature() {}\n"), 0o644))
	worktreeGit(s.T(), task.WorktreePath, "add", "-A")
	worktreeGit(s.T(), task.WorktreePath, "commit", "-m", "add feature")

	diff := env.Client.GetGitDiff(task.ID)
	require.GreaterOrEqual(s.T(), len(diff.Files), 1)

	found := false
	for _, f := range diff.Files {
		if f.Name == "new_feature.go" {
			found = true
			assert.GreaterOrEqual(s.T(), f.Hunks, 1)
			assert.Greater(s.T(), f.Added, 0)
		}
	}
	assert.True(s.T(), found, "expected new_feature.go in diff")
}

func worktreeGit(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
