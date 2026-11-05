package hierarchy

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrParentLocked is returned when a merge targets a locked parent workspace.
// Locked parents (protected branches) may not receive local merges.
var ErrParentLocked = problem.New(problem.Conflict, "usecases: parent is locked")

// ErrRebaseNonLeaf is returned when a rebase-strategy merge is requested for a
// child that has its own children; rebasing would rewrite SHAs the descendants
// fork from, so it is forbidden for non-leaf children.
var ErrRebaseNonLeaf = problem.New(problem.Conflict, "usecases: rebase forbidden for non-leaf child")

// ErrChildHasChildren is returned when a re-parent is requested for a child that
// is not a leaf; only leaf workspaces may be re-parented.
var ErrChildHasChildren = problem.New(problem.Conflict, "usecases: child has children")

// ErrCrossRepoWorktreeMove is returned when a re-parent would move a worktree-owning child under a parent
// in a DIFFERENT repo: that is a different checkout, not a reparent.
var ErrCrossRepoWorktreeMove = problem.New(problem.Conflict, "usecases: cannot move a worktree across repos")

// ErrSelfParent is returned when a re-parent targets the child itself, a self-loop that would detach the
// node and make it unreparentable; rejected before any git work.
var ErrSelfParent = problem.New(problem.Conflict, "usecases: cannot reparent a workspace onto itself")

// ErrWorkspaceLocked is returned when a cascade delete targets a locked root, checked before any
// destructive step so it fails cleanly. Handlers map it to HTTP 409.
var ErrWorkspaceLocked = problem.New(problem.Conflict, "workspace is locked")

// ErrWorkspaceIsDefault is returned when a cascade delete targets a repo's
// main checkout: it is never a child worktree, and only deleting the repo
// takes it. Handlers map it to HTTP 409.
var ErrWorkspaceIsDefault = problem.New(problem.Conflict, "workspace is the repository's main checkout")

// ErrWorkspaceIsHome is returned when a cascade delete targets a project's
// home: only deleting the project takes it. Handlers map it to HTTP 409.
var ErrWorkspaceIsHome = problem.New(problem.Conflict, "workspace is the project's home")

// ErrBranchWorkspaceExists is returned when a create would make a second workspace for a branch already
// held in the repo (one worktree per branch), before any git work. Handlers map it to HTTP 409.
var ErrBranchWorkspaceExists = problem.New(problem.Conflict, "usecases: a workspace already exists for this branch")

// ErrParentUnprovisioned is returned when a parent-tip op targets a placeholder parent with no
// WorktreePath yet, before any git op could run against "". Handlers map it to HTTP 409.
var ErrParentUnprovisioned = problem.New(problem.Conflict, "usecases: parent branch is not yet provisioned")

// ErrRenameUnmanagedWorkspace is returned when a rename targets an adopted checkout outside crowbar home:
// the user's own directory, which Crowbar does not own. Handlers map it to HTTP 409.
var ErrRenameUnmanagedWorkspace = problem.New(problem.Conflict,
	"usecases: cannot rename the branch of an adopted checkout",
)

// ErrRenameTargetExists is returned when a rename's destination directory is occupied, before any git
// work, so it never clobbers or nests another tree. Handlers map it to HTTP 409.
var ErrRenameTargetExists = problem.New(problem.Conflict, "usecases: a directory already exists for the new branch")

// ErrBranchStillHeld is returned by RetryProvision while a live worktree still holds the protected
// branch: the user must detach the holder first.
var ErrBranchStillHeld = problem.New(problem.Conflict, "usecases: branch is still held; detach the holder first")

// ErrBranchHeldByManagedWorkspace is returned by RetryProvision when ANOTHER managed worktree holds the
// branch. Detaching would strand that workspace, so the retry succeeds only once it is gone.
var ErrBranchHeldByManagedWorkspace = problem.New(problem.Conflict,
	"usecases: branch is checked out in another Crowbar workspace",
)

// ErrWorkspaceWorking is returned when a reparent's subtree owns a working chat (the chat tree's
// guardNotWorking rule), before any git work, so a turn is never displaced.
var ErrWorkspaceWorking = problem.New(problem.Conflict, "usecases: workspace has a working chat")
