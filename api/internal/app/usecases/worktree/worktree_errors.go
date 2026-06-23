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

// ErrNewParentLocked is returned when a re-parent targets a locked new parent
// workspace (07 §4).
var ErrNewParentLocked = errors.New("usecases: new parent is locked")

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
