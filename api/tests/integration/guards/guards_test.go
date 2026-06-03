//go:build integration

package guards

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestGuards(t *testing.T) { suite.Run(t, new(GuardsSuite)) }

type GuardsSuite struct{ kit.IntegrationSuite }

// TestCreateTaskNonexistentRepo verifies that creating a task with a
// nonexistent repository ID returns 404.
func (s *GuardsSuite) TestCreateTaskNonexistentRepo() {
	resp := s.Env.Client.RawDo("POST", "/api/v0/repositories/nonexistent-repo-id/tasks", kit.CreateTaskRequest{
		Title:      "t",
		BranchName: "b",
		BaseBranch: "main",
	})
	defer resp.Body.Close()
	assert.Equal(s.T(), http.StatusNotFound, resp.StatusCode)
}

// TestCreateTaskWithInvalidFlowYAML verifies that a flow YAML path with
// validation errors returns 400 with details.
func (s *GuardsSuite) TestCreateTaskWithInvalidFlowYAML() {
	yaml := `name: broken-flow
version: "1.0"
states:
  - name: start
    transitions:
      - on: go
        to: nowhere
`
	dir := s.T().TempDir()
	p := filepath.Join(dir, "broken.yaml")
	require.NoError(s.T(), os.WriteFile(p, []byte(yaml), 0o644))

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(s.Env.Client)
	repo := kit.DefaultRepository(s.Env.Client, project.ID, gitRepo.Path())

	resp := s.Env.Client.RawDo("POST", "/api/v0/repositories/"+repo.ID+"/tasks", kit.CreateTaskRequest{
		Title:      "bad flow task",
		BranchName: "bad-flow-branch",
		BaseBranch: "main",
		FlowPath:   p,
	})
	defer resp.Body.Close()
	assert.Equal(s.T(), http.StatusBadRequest, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(s.T(), string(body), "error")
}

// TestForceTransitionToUnknownState verifies that force-transitioning to a
// state not in the flow returns 400.
func (s *GuardsSuite) TestForceTransitionToUnknownState() {
	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(s.Env.Client)
	repo := kit.DefaultRepository(s.Env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(s.Env.Client, repo.ID, "guard-transition-branch")

	resp := s.Env.Client.RawDo("POST", "/api/v0/tasks/"+task.ID+"/force-transition", map[string]string{
		"to_state": "definitely_not_a_real_state",
	})
	defer resp.Body.Close()
	assert.Equal(s.T(), http.StatusBadRequest, resp.StatusCode)
}

// TestGetNonexistentTask verifies that getting a nonexistent task returns 404.
func (s *GuardsSuite) TestGetNonexistentTask() {
	resp := s.Env.Client.RawDo("GET", "/api/v0/tasks/task-does-not-exist", nil)
	defer resp.Body.Close()
	assert.Equal(s.T(), http.StatusNotFound, resp.StatusCode)
}
