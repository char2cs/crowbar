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
