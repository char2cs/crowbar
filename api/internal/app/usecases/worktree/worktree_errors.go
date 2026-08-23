package worktree

import "errors"

// ErrParentLocked is returned when a merge targets a locked parent workspace.
// Locked parents (protected branches) may not receive local merges (07 §3.1).
var ErrParentLocked = errors.New("usecases: parent is locked")

// ErrRebaseNonLeaf is returned when a rebase-strategy merge is requested for a
// child that has its own children; rebasing would rewrite SHAs the descendants
// fork from, so it is forbidden for non-leaf children (07 §3.1).
var ErrRebaseNonLeaf = errors.New("usecases: rebase forbidden for non-leaf child")

// ErrChildHasChildren is returned when a re-parent is requested for a child that
// is not a leaf; only leaf workspaces may be re-parented (07 §4).
var ErrChildHasChildren = errors.New("usecases: child has children")

// ErrSelfParent is returned when a re-parent targets the child itself. A
// workspace cannot be its own parent: the self-loop both detaches the node in
// the tree and makes it permanently unreparentable (it would count as its own
// child in the leaf check), so it is rejected before any git work.
var ErrSelfParent = errors.New("usecases: cannot reparent a workspace onto itself")

// ErrWorkspaceLocked is returned when a cascade delete targets a locked root
// workspace. The guard runs before any destructive side effect (worktree
// removal, branch delete), so a locked workspace is rejected cleanly instead
// of failing midway with a raw git error. Handlers map it to HTTP 409.
var ErrWorkspaceLocked = errors.New("workspace is locked")

// ErrBranchWorkspaceExists is returned when a create would produce a second
// workspace for a branch that a non-deleted workspace already holds in the repo.
// A branch can be checked out in at most one worktree, so the repo keeps at most
// one workspace per branch (the default workspace reserves the default branch).
// The guard runs before any git work, so a duplicate is rejected cleanly instead
// of failing midway with a raw git error. Handlers map it to HTTP 409.
var ErrBranchWorkspaceExists = errors.New("usecases: a workspace already exists for this branch")

// ErrParentUnprovisioned is returned when a parent-tip op (merge-into-parent,
// reparent-onto, rebase-onto-parent) targets a placeholder parent — a locked row
// whose WorktreePath is still empty because its protected branch has not been
// materialised yet. The guard runs before any RevParse so no git op can run
// against "". Handlers map it to HTTP 409 (spec §3.4/B2).
var ErrParentUnprovisioned = errors.New("usecases: parent branch is not yet provisioned")

// ErrRenameUnmanagedWorkspace is returned when a branch rename targets a
// workspace whose worktree is NOT under crowbar home — an adopted checkout (the
// repo home / project home) living at the user's own directory. Crowbar derives
// managed worktree paths from branch names, but it does not own that directory
// and must never relocate it, so the rename is refused rather than silently
// moving a user's folder. Handlers map it to HTTP 409.
var ErrRenameUnmanagedWorkspace = errors.New(
	"usecases: cannot rename the branch of an adopted checkout",
)

// ErrRenameTargetExists is returned when the directory a branch rename would
// move the workspace into is already occupied. The guard runs before any git
// work: renaming onto a live path would either clobber another workspace's tree
// or nest this one inside it. Handlers map it to HTTP 409.
var ErrRenameTargetExists = errors.New("usecases: a directory already exists for the new branch")

// ErrBranchStillHeld is returned by RetryProvision when the protected branch is
// still held by a live worktree (the repo home or an external checkout) that was
// not freed first: the user must detach the holder before a retry can succeed
// (spec §3.3/§3.7).
var ErrBranchStillHeld = errors.New("usecases: branch is still held; detach the holder first")

// ErrBranchHeldByManagedWorkspace is returned by RetryProvision when the branch
// is checked out by ANOTHER Crowbar-managed worktree — the same repo folder
// imported under a second project, or a worktree outlasting the repo it was
// created for. Detaching is not on offer here: freeing the branch would strand
// the workspace that owns it, so this is not ErrBranchStillHeld's "detach the
// holder first". The retry can only succeed once that workspace is gone.
var ErrBranchHeldByManagedWorkspace = errors.New(
	"usecases: branch is checked out in another Crowbar workspace",
)
