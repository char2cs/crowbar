package main

import (
	"context"
	"fmt"
)

const seedRepoName = "checkout"

type repoDTO struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
}

// pickRepo filters on the project id as well as the name because
// GET /v0/projects/:projectId/repos answers with EVERY repo the daemon knows,
// not just the path project's. Matching on name alone would make the seed adopt
// an unrelated project's repo that happens to share the name.
func pickRepo(
	projectID string,
) func([]repoDTO) (repoDTO, bool) {
	return func(list []repoDTO) (repoDTO, bool) {
		for _, r := range list {
			if r.Name == seedRepoName && r.ProjectID == projectID {
				return r, true
			}
		}
		return repoDTO{}, false
	}
}

// ensureRepo imports the fixture repo under the seed project, reusing an
// existing one. Import is the step that provisions the locked base-branch
// worktree, so a second import would fight the first one for the branch.
func ensureRepo(
	ctx context.Context,
	d *daemon,
	projectID string,
	repoPath string,
) (repoDTO, bool, error) {
	path := fmt.Sprintf("%s/%s/repos", projectsPath, projectID)
	pick := pickRepo(projectID)
	existing, err := getData[[]repoDTO](ctx, d, "list repos", path)
	if err != nil {
		return repoDTO{}, false, err
	}
	if found, ok := pick(existing); ok {
		return found, false, nil
	}
	body := map[string]any{
		"name":          seedRepoName,
		"path":          repoPath,
		"defaultBranch": seedBaseBranch,
	}
	if err := d.postAccepted(ctx, "import repo", path, body); err != nil {
		return repoDTO{}, false, err
	}
	created, err := waitFor(ctx, d, "the seed repo", path, pick)
	return created, true, err
}
