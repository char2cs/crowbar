package domain

// WorkspaceProvisioning is what stands behind a workspace's WorktreePath. It is
// recorded at creation and changed only by ProvisionInPlace, never inferred from the
// path, IsDefault or Kind.
type WorkspaceProvisioning string

const (
	// WorkspaceProvisioned is a managed worktree Crowbar checked out under its
	// home; Crowbar removes it with the workspace.
	WorkspaceProvisioned WorkspaceProvisioning = "provisioned"
	// WorkspacePlaceholder has no worktree of its own yet: another checkout
	// (HeldByPath) holds its branch. Nothing may run git in it or fork from it
	// until a retry provisions it in place, keeping its id.
	WorkspacePlaceholder WorkspaceProvisioning = "placeholder"
	// WorkspaceShared is a checkout the user owns and Crowbar adopted in place —
	// a repo's or a project's home. Crowbar never removes it.
	WorkspaceShared WorkspaceProvisioning = "shared"
)

// HasWorktree reports whether the workspace has a checkout to run git in.
func (p WorkspaceProvisioning) HasWorktree() bool {
	return p == WorkspaceProvisioned || p == WorkspaceShared
}
