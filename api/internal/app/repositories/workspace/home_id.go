package workspace

import "github.com/google/uuid"

// homeWorkspaceNamespace is this package's own RFC 4122 namespace, so an id
// minted here never collides with one derived elsewhere from the same name.
var homeWorkspaceNamespace = uuid.MustParse("f9a1b2c3-2026-4a1a-8b1c-c70de5e8f001")

// ProjectHomeID derives a project's home workspace id from its project id, so two
// concurrent creates of a project's home target one aggregate and asynx refuses the
// second (CreateWorkspace's Validate) instead of minting a duplicate home.
func ProjectHomeID(projectID string) string {
	return uuid.NewSHA1(homeWorkspaceNamespace, []byte(projectID)).String()
}

// RepoHomeID derives a repo's home (IsDefault) workspace id from the repo id, for
// ProjectHomeID's reason; namespaced apart from the project home's.
func RepoHomeID(repoID string) string {
	return uuid.NewSHA1(homeWorkspaceNamespace, []byte("repo-home:"+repoID)).String()
}
