// Package branchimport describes an EXISTING git branch adopted as a workspace. It sits above
// usecases/chat/internal so the chat tree's verb and the shared mocks can both name it; the hierarchy and
// project sides declare their own inputs and the container translates.
package branchimport

// Spec describes the existing branch an import adopts. Every field is explicit: an import rooted at the
// repo has no parent to inherit from. ParentWorkspaceID is the git lineage parent, separate from sidebar
// placement; empty hangs the branch at the repo root.
type Spec struct {
	RepoID            string
	ProjectID         string
	RepoPath          string
	RemoteURL         string
	Branch            string
	ParentWorkspaceID string
	ParentBranch      string
	// ForceLocked marks the workspace locked regardless of what the provider
	// says about the branch, for a caller that already knows it is importing a
	// branch Crowbar must not let anyone commit onto.
	ForceLocked bool
}
