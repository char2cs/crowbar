package branchimport

type Spec struct {
	RepoID            string
	ProjectID         string
	RepoPath          string
	RemoteURL         string
	Branch            string
	ParentWorkspaceID string
	ParentBranch      string
	ForceLocked       bool
}
