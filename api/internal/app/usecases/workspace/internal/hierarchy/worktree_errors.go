package hierarchy

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var ErrParentLocked = problem.New(problem.Conflict, "usecases: parent is locked")

var ErrRebaseNonLeaf = problem.New(problem.Conflict, "usecases: rebase forbidden for non-leaf child")

var ErrChildHasChildren = problem.New(problem.Conflict, "usecases: child has children")

var ErrCrossRepoWorktreeMove = problem.New(problem.Conflict, "usecases: cannot move a worktree across repos")

var ErrSelfParent = problem.New(problem.Conflict, "usecases: cannot reparent a workspace onto itself")

var ErrWorkspaceLocked = problem.New(problem.Conflict, "workspace is locked")

var ErrWorkspaceIsDefault = problem.New(problem.Conflict, "workspace is the repository's main checkout")

var ErrWorkspaceIsHome = problem.New(problem.Conflict, "workspace is the project's home")

var ErrBranchWorkspaceExists = problem.New(problem.Conflict, "usecases: a workspace already exists for this branch")

var ErrParentUnprovisioned = problem.New(problem.Conflict, "usecases: parent branch is not yet provisioned")

var ErrRenameUnmanagedWorkspace = problem.New(problem.Conflict,
	"usecases: cannot rename the branch of an adopted checkout",
)

var ErrRenameTargetExists = problem.New(problem.Conflict, "usecases: a directory already exists for the new branch")

var ErrBranchStillHeld = problem.New(problem.Conflict, "usecases: branch is still held; detach the holder first")

var ErrBranchHeldByManagedWorkspace = problem.New(problem.Conflict,
	"usecases: branch is checked out in another Crowbar workspace",
)

var ErrWorkspaceWorking = problem.New(problem.Conflict, "usecases: workspace has a working chat")

var ErrMergeSourceWorking = problem.New(problem.ChatBusy, "the chat is working: stop it first")
