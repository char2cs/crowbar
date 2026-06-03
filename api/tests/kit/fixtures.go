//go:build integration || manual

package kit

import (
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DefaultProject creates a Project with a generated unique name.
func DefaultProject(
	c *Client,
) domain.Project {
	return c.CreateProject(CreateProjectRequest{
		Name: fmt.Sprintf("test-%d", time.Now().UnixNano()),
	})
}

// DefaultRepository creates a Repository under project using the real git repo
// at repoPath.
func DefaultRepository(
	c *Client,
	projectID string,
	repoPath string,
) domain.Repository {
	return c.CreateRepository(projectID, CreateRepositoryRequest{
		Name: fmt.Sprintf("repo-%d", time.Now().UnixNano()),
		Path: repoPath,
	})
}

// DefaultTask creates a Task on the given repository using the builtin
// feature-development flow. branch is used as both the task title and branch
// name; base_branch defaults to "main".
func DefaultTask(
	c *Client,
	repoID string,
	branch string,
) domain.Task {
	return c.CreateTask(repoID, CreateTaskRequest{
		Title:      branch,
		BranchName: branch,
		BaseBranch: "main",
		FlowPath:   "",
	})
}
