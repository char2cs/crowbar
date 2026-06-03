//go:build integration

package worktree

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestWorktree(t *testing.T) { suite.Run(t, new(WorktreeSuite)) }

type WorktreeSuite struct{ kit.IntegrationSuite }

// TestCreateTaskCreatesWorktreeOnDisk verifies that creating a task causes git
// worktree add to run and the resulting directory to exist on disk.
func (s *WorktreeSuite) TestCreateTaskCreatesWorktreeOnDisk() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "worktree-create-branch")

	// The worktree path must exist on disk.
	info, err := os.Stat(task.WorktreePath)
	require.NoError(s.T(), err, "worktree directory should exist")
	assert.True(s.T(), info.IsDir())

	// git worktree list should show our branch.
	out, err := gitWorktreeList(gitRepo.Path())
	require.NoError(s.T(), err)
	assert.Contains(s.T(), out, "worktree-create-branch")
}

// TestArchiveTaskRemovesWorktree verifies that archiving a task removes the
// worktree directory from disk.
func (s *WorktreeSuite) TestArchiveTaskRemovesWorktreeOnDisk() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "worktree-archive-branch")

	// Confirm worktree exists before archive.
	_, err := os.Stat(task.WorktreePath)
	require.NoError(s.T(), err)

	env.Client.ArchiveTask(task.ID)

	// Worktree directory should be gone.
	_, err = os.Stat(task.WorktreePath)
	assert.True(s.T(), os.IsNotExist(err), "worktree directory should be removed after archive")
}

// TestDuplicateBranchNameFails verifies that creating a second task with the
// same branch name on the same repository fails.
func (s *WorktreeSuite) TestDuplicateBranchNameFails() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())

	kit.DefaultTask(env.Client, repo.ID, "duplicate-branch")

	// Second task with the same branch name should fail.
	resp := env.Client.RawDo("POST", "/api/v0/repositories/"+repo.ID+"/tasks", kit.CreateTaskRequest{
		Title:      "second task",
		BranchName: "duplicate-branch",
		BaseBranch: "main",
		FlowPath:   "",
	})
	defer resp.Body.Close()
	assert.NotEqual(s.T(), 201, resp.StatusCode, "expected error for duplicate branch name")
}

func gitWorktreeList(
	dir string,
) (string, error) {
	cmd := exec.Command("git", "worktree", "list")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
