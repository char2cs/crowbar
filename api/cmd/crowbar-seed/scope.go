package main

import "fmt"

// scope is the project/repo/workspace triple every workspace-scoped v0 route is
// nested under.
type scope struct {
	projectID   string
	repoID      string
	workspaceID string
}

func (s scope) path(
	suffix string,
) string {
	return fmt.Sprintf(
		"/v0/projects/%s/repos/%s/workspaces/%s%s",
		s.projectID, s.repoID, s.workspaceID, suffix,
	)
}

func repoScopePath(
	projectID string,
	repoID string,
	suffix string,
) string {
	return fmt.Sprintf("/v0/projects/%s/repos/%s%s", projectID, repoID, suffix)
}
