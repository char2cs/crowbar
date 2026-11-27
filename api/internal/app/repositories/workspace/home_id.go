package workspace

import "github.com/google/uuid"

var homeWorkspaceNamespace = uuid.MustParse("f9a1b2c3-2026-4a1a-8b1c-c70de5e8f001")

func ProjectHomeID(projectID string) string {
	return uuid.NewSHA1(homeWorkspaceNamespace, []byte(projectID)).String()
}

func RepoHomeID(repoID string) string {
	return uuid.NewSHA1(homeWorkspaceNamespace, []byte("repo-home:"+repoID)).String()
}
