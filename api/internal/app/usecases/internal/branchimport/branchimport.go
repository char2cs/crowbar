// Package branchimport carries the description of an EXISTING git branch being
// adopted as a workspace, shared by the two sides of that create.
//
// It is its own package for a visibility reason, not a taxonomic one. The chat
// tree declares the verb (tree.Agent.SpawnChatWithImportedWorktree) and lives
// under usecases/chat/internal, which nothing outside that feature may import —
// including the shared test doubles in usecases/mocks, which have to satisfy
// that same interface. A type both can name has to sit above both, and this is
// the narrowest place that is.
//
// The consumers on the OTHER side of the container — the worktree hierarchy and
// the project import — deliberately do NOT use this type. Each declares its own
// input beside the port it needs (hierarchy.ImportedBranch), because each is a
// consumer naming the narrow slice it wants; the container translates. This
// type is the chat feature's own vocabulary, shared only with the doubles that
// stand in for it.
package branchimport

// Spec describes the existing branch an import adopts.
//
// Every field is named outright rather than left to the worktree create's
// parent-inherited defaulting: a caller that imports a branch already knows
// which repository it found it in, and inheriting instead would resolve the
// repo from a parent workspace an import rooted at the repo does not have.
//
// ParentWorkspaceID is the GIT LINEAGE parent (domain.Workspace.ParentID) and
// is deliberately separate from where the new chat is PLACED in the sidebar:
// placement is resolved from the chat that owns that workspace, and the two
// have been independently written fields since long before this. Empty means
// the branch hangs at the repo root.
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
